/*****************************************************************************
 **
 ** PerFeediem
 ** https://github.com/melllvar/perfeediem
 ** Copyright (C) 2013 Akop Karapetyan
 **
 ** This program is free software; you can redistribute it and/or modify
 ** it under the terms of the GNU General Public License as published by
 ** the Free Software Foundation; either version 2 of the License, or
 ** (at your option) any later version.
 **
 ** This program is distributed in the hope that it will be useful,
 ** but WITHOUT ANY WARRANTY; without even the implied warranty of
 ** MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 ** GNU General Public License for more details.
 **
 ** You should have received a copy of the GNU General Public License
 ** along with this program; if not, write to the Free Software
 ** Foundation, Inc., 675 Mass Ave, Cambridge, MA 02139, USA.
 **
 ******************************************************************************
 */
 
package perfeediem

import (
  "appengine"
  "appengine/blobstore"
  "appengine/datastore"
  "appengine/taskqueue"
  "appengine/urlfetch"
  "appengine/user"
  "encoding/json"
  "fmt"
  "io/ioutil"
  "net/url"
  "net/http"
  "opml"
  "regexp"
  "rss"
  "strings"
  "unicode/utf8"
)

var validProperties = map[string]bool {
  "unread": true,
  "read":   true,
  "star":   true,
  "like":   true,
}

func registerJson() {
  http.HandleFunc("/subscriptions", subscriptions)
  http.HandleFunc("/entries",       entries)
  http.HandleFunc("/setProperty",   setProperty)
  http.HandleFunc("/subscribe",     subscribe)
  http.HandleFunc("/import",        importOpml)
  http.HandleFunc("/authUpload",    authUpload)
  http.HandleFunc("/createFolder",  createFolder)
}

func _l(format string, v ...interface {}) string {
  // FIXME
  return fmt.Sprintf(format, v...)
}

func writeObject(w http.ResponseWriter, obj interface{}) {
  w.Header().Set("Content-type", "application/json; charset=utf-8")

  bf, _ := json.Marshal(obj)
  w.Write(bf)
}

func feedURLFromLink(c appengine.Context, url string) string {
  q := datastore.NewQuery("Feed").Filter("Link =", url).Limit(1)

  var feeds []*Feed
  if _, err := q.GetAll(c, &feeds); err == nil && len(feeds) > 0 {
    return feeds[0].URL
  } else if err != nil && err != datastore.ErrNoSuchEntity {
    c.Errorf("Error searching for feed (URL %s): %s", url, err)
    return ""
  }

  // Add/remove 'www' and try again
  re := regexp.MustCompile(`://www\.`)
  if re.MatchString(url) {
    url = re.ReplaceAllString(url, "://")
  } else {
    re = regexp.MustCompile(`://`)
    url = re.ReplaceAllString(url, "://www.")
  }

  q = datastore.NewQuery("Feed").Filter("Link =", url).Limit(1)
  if _, err := q.GetAll(c, &feeds); err == nil && len(feeds) > 0 {
    return feeds[0].URL
  } else if err != nil && err != datastore.ErrNoSuchEntity {
    c.Errorf("Error searching for feed (URL %s): %s", url, err)
    return ""
  }

  return ""
}

func feedURLFromHTML(html string) string {
  tagRe := regexp.MustCompile(`<link(?:\s+\w+\s*=\s*(?:"[^"]*"|'[^']'))+\s*/?>`)
  attrRe := regexp.MustCompile(`\b(?P<key>\w+)\s*=\s*(?:"(?P<value>[^"]*)"|'(?P<value>[^'])')`)

  for _, linkTag := range tagRe.FindAllString(html, -1) {
    link := make(map[string]string)
    for _, attr := range attrRe.FindAllStringSubmatch(linkTag, -1) {
      key := strings.ToLower(attr[1])
      if attr[2] != "" {
        link[key] = strings.ToLower(attr[2])
      } else if attr[3] != "" {
        link[key] = strings.ToLower(attr[3])
      }
    }

    if link["rel"] == "alternate" && link["type"] == "application/rss+xml" {
      return link["href"]
    }
  }

  return ""
}

func authorize(c appengine.Context, r *http.Request, w http.ResponseWriter) (*datastore.Key, error) {
  u := user.Current(c)
  if u == nil {
    err := NewReadableErrorWithCode(_l("Your session has expired. Please sign in."), http.StatusUnauthorized, nil)
    return nil, err
  }

  return datastore.NewKey(c, "User", u.ID, 0, nil), nil
}

func subscriptions(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  var userKey *datastore.Key
  if u, err := authorize(c, r, w); err != nil {
    writeError(c, w, err)
    return
  } else {
    userKey = u
  }

  var subscriptions []*Subscription
  var subscriptionKeys []*datastore.Key

  q := datastore.NewQuery("Subscription").Ancestor(userKey).Order("Title").Limit(1000)
  if subKeys, err := q.GetAll(c, &subscriptions); err != nil {
    writeError(c, w, err)
    return
  } else if subscriptions == nil {
    subscriptions = make([]*Subscription, 0)
  } else {
    subscriptionKeys = subKeys
  }

  totalUnreadCount := 0
  feedKeys := make([]*datastore.Key, len(subscriptions))
  for i, subscription := range subscriptions {
    feedKeys[i] = subscription.Feed
    totalUnreadCount += subscription.UnreadCount
  }

  feeds := make([]Feed, len(subscriptions))
  if err := datastore.GetMulti(c, feedKeys, feeds); err != nil {
    writeError(c, w, err)
    return
  }

  for i, subscription := range subscriptions {
    feed := feeds[i]

    subscription.ID = subscriptionKeys[i].StringID()
    subscription.Link = feed.Link
  }

  allItems := Subscription {
    ID: "",
    Link: "",

    Title: _l("Subscriptions"),
    UnreadCount: totalUnreadCount,
  }

  writeObject(w, append([]*Subscription { &allItems }, subscriptions...))
}

func getEntries(c appengine.Context, ancestorKey *datastore.Key, filterProperty string, continueFrom *string) ([]SubEntry, error) {
  q := datastore.NewQuery("SubEntry").Ancestor(ancestorKey).Order("-Published")
  if filterProperty != "" {
    q = q.Filter("Properties = ", filterProperty)
  }

  if *continueFrom != "" {
    if cursor, err := datastore.DecodeCursor(*continueFrom); err == nil {
      q = q.Start(cursor)
    }
  }

  subEntries := make([]SubEntry, 40)
  entryKeys := make([]*datastore.Key, 40)

  t := q.Run(c)

  var readCount int
  for readCount = 0; readCount < 40; readCount++ {
    subEntry := &subEntries[readCount]

    if _, err := t.Next(subEntry); err != nil && err == datastore.Done {
      break
    } else if err != nil {
      return nil, err
    }

    entryKey := subEntries[readCount].Entry
    entryKeys[readCount] = entryKey
    subEntry.ID = entryKey.StringID()
    subEntry.Source = entryKey.Parent().StringID()
  }

  *continueFrom = ""
  if readCount >= 40 {
    if cursor, err := t.Cursor(); err == nil {
      *continueFrom = cursor.String()
    }
  }

  subEntries = subEntries[:readCount]
  entryKeys = entryKeys[:readCount]

  entries := make([]Entry, readCount)
  if err := datastore.GetMulti(c, entryKeys, entries); err != nil {
    return nil, err
  }

  for i, _ := range subEntries {
    subEntries[i].Details = &entries[i]
  }

  return subEntries, nil
}

func entries(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  var userKey *datastore.Key
  if u, err := authorize(c, r, w); err != nil {
    writeError(c, w, err)
    return
  } else {
    userKey = u
  }

  var ancestorKey *datastore.Key
  if subscriptionID := r.FormValue("subscription"); subscriptionID == "" {
    ancestorKey = userKey
  } else {
    ancestorKey = datastore.NewKey(c, "Subscription", subscriptionID, 0, userKey)
  }

  continueFrom := r.FormValue("continue")
  filterProperty := r.FormValue("filter")

  if !validProperties[filterProperty] {
    filterProperty = ""
  }

  response := make(map[string]interface{})
  if entries, err := getEntries(c, ancestorKey, filterProperty, &continueFrom); err != nil {
    writeError(c, w, err)
    return
  } else {
    response["entries"] = entries
    if continueFrom != "" {
      response["continue"] = continueFrom
    }
  }

  writeObject(w, response)
}

func setProperty(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  subEntryID := r.FormValue("entry")
  subscriptionID := r.FormValue("subscription")

  if subEntryID == "" || subscriptionID == "" {
    writeError(c, w, NewReadableError(_l("Article not found"), nil))
    return
  }

  propertyName := r.FormValue("property")
  setProp := r.FormValue("set") == "true"

  if !validProperties[propertyName] {
    writeError(c, w, NewReadableError(_l("Property not valid"), nil))
    return
  }

  var userKey *datastore.Key
  if u, err := authorize(c, r, w); err != nil {
    writeError(c, w, err)
    return
  } else {
    userKey = u
  }

  subscriptionKey := datastore.NewKey(c, "Subscription", subscriptionID, 0, userKey)
  subEntryKey := datastore.NewKey(c, "SubEntry", subEntryID, 0, subscriptionKey)

  subEntry := new(SubEntry)
  if err := datastore.Get(c, subEntryKey, subEntry); err != nil {
    writeError(c, w, NewReadableError(_l("Article not found"), &err))
    return
  }

  // Convert set property list to a map
  propertyMap := make(map[string]bool)
  for _, property := range subEntry.Properties {
    propertyMap[property] = true
  }

  unreadDelta := 0
  writeChanges := false

  // 'read' and 'unread' are mutually exclusive
  if propertyName == "read" {
    if propertyMap[propertyName] && !setProp {
      delete(propertyMap, "read")
      propertyMap["unread"] = true
      unreadDelta = 1
    } else if !propertyMap[propertyName] && setProp {
      delete(propertyMap, "unread")
      propertyMap["read"] = true
      unreadDelta = -1
    }
    writeChanges = unreadDelta != 0
  } else if propertyName == "unread" {
    if propertyMap[propertyName] && !setProp {
      delete(propertyMap, "unread")
      propertyMap["read"] = true
      unreadDelta = -1
    } else if !propertyMap[propertyName] && setProp {
      delete(propertyMap, "read")
      propertyMap["unread"] = true
      unreadDelta = 1
    }
    writeChanges = unreadDelta != 0
  } else {
    if propertyMap[propertyName] && !setProp {
      delete(propertyMap, propertyName)
      writeChanges = true
    } else if !propertyMap[propertyName] && setProp {
      propertyMap[propertyName] = true
      writeChanges = true
    }
  }

  if writeChanges {
    subEntry.Properties = make([]string, len(propertyMap))
    i := 0
    for key, _ := range propertyMap {
      subEntry.Properties[i] = key
      i++
    }

    if _, err := datastore.Put(c, subEntryKey, subEntry); err != nil {
      writeError(c, w, NewReadableError(_l("Error updating article"), &err))
      return
    }

    if unreadDelta != 0 {
      // Update unread counts - not critical
      subscription := new(Subscription)
      if err := datastore.Get(c, subscriptionKey, subscription); err != nil {
        c.Errorf("Unread count update failed: subscription fetch error (%s)", err)
      } else {
        subscription.UnreadCount += unreadDelta
        if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
          c.Errorf("Unread count update failed: subscription write error (%s)", err)
        }
      }
    }
  }

  writeObject(w, subEntry.Properties)
}

func subscribe(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  var userKey *datastore.Key
  if u, err := authorize(c, r, w); err != nil {
    writeError(c, w, err)
    return
  } else {
    userKey = u
  }

  subscriptionURL := strings.TrimSpace(r.PostFormValue("url"))
  if subscriptionURL == "" {
    writeError(c, w, NewReadableError(_l("Missing URL"), nil))
    return
  }

  var feedURL string
  if url := feedURLFromLink(c, subscriptionURL); url != "" {
    feedURL = url
  } else {
    feedURL = subscriptionURL
  }

  subscriptionKey := datastore.NewKey(c, "Subscription", feedURL, 0, userKey)
  subscription := new(Subscription)

  if err := datastore.Get(c, subscriptionKey, subscription); err == nil {
    writeObject(w, map[string]string { "message": _l("You are already subscribed to %s", subscription.Title) })
    return
  } else if err != datastore.ErrNoSuchEntity {
    writeError(c, w, err)
    return
  }

  feedKey := datastore.NewKey(c, "Feed", feedURL, 0, nil)
  if err := datastore.Get(c, feedKey, nil); err == nil {
    // Already have the feed
  } else if err == datastore.ErrNoSuchEntity {
    // Don't have the feed - fetch it
    client := urlfetch.Client(c)
    if response, err := client.Get(feedURL); err != nil {
      writeError(c, w, NewReadableError(_l("An error occurred while downloading the feed"), &err))
      return
    } else {
      defer response.Body.Close()
      
      var body string
      if bytes, err := ioutil.ReadAll(response.Body); err != nil {
        writeError(c, w, NewReadableError(_l("An error occurred while downloading the feed"), &err))
        return
      } else {
        body = string(bytes)
      }

      reader := strings.NewReader(body) // FIXME
      if _, err := rss.UnmarshalStream(feedURL, reader); err != nil {
        // Parse failed. Assume it's HTML and try to pull out an RSS link
        if linkURL := feedURLFromHTML(body); linkURL == "" {
          writeError(c, w, NewReadableError(_l("RSS content not found"), &err))
          return
        } else {
          feedURL = linkURL
        }
      }
    }
  } else {
    // Some other error
    writeError(c, w, err)
    return
  }

  user := user.Current(c)
  task := taskqueue.NewPOSTTask("/tasks/subscribe", url.Values {
    "url": { feedURL },
    "userID": { user.ID },
  })

  if _, err := taskqueue.Add(c, task, ""); err != nil {
    writeError(c, w, NewReadableError(_l("Subscription may already have been queued"), &err))
    return
  }

  writeObject(w, map[string]string { "message": _l("Your subscription has been queued for addition.") })
}

func importOpml(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  blobs, _, err := blobstore.ParseUpload(r)
  if err != nil {
    writeError(c, w, NewReadableError(_l("Error receiving file"), &err))
    return
  }

  var blobKey appengine.BlobKey
  if blobInfos := blobs["opml"]; len(blobInfos) == 0 {
    writeError(c, w, NewReadableError(_l("File not uploaded"), nil))
    return
  } else {
    blobKey = blobInfos[0].BlobKey
    reader := blobstore.NewReader(c, blobKey)

    var doc opml.Document
    if err := opml.Parse(reader, &doc); err != nil {
      writeError(c, w, NewReadableError(_l("Error reading OPML file"), &err))

      // Remove the blob
      if err := blobstore.Delete(c, blobKey); err != nil {
        c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
      }
      return
    }
  }

  user := user.Current(c)
  task := taskqueue.NewPOSTTask("/tasks/import", url.Values {
    "opmlBlobKey": { string(blobKey) },
    "userID": { user.ID },
  })

  if _, err := taskqueue.Add(c, task, ""); err != nil {
    writeError(c, w, NewReadableError(_l("Error initiating import"), &err))
    
    // Remove the blob
    if err := blobstore.Delete(c, blobKey); err != nil {
      c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
    }
    return
  }

  writeObject(w, map[string]string { "message": _l("Your subscriptions have been queued for addition.") })
}

func authUpload(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  uploadURL, err := blobstore.UploadURL(c, "/import", nil)
  if err != nil {
    writeError(c, w, NewReadableError(_l("Error initiating file upload"), &err))
    return
  }

  writeObject(w, map[string]string { 
    "uploadUrl": uploadURL.String(),
  })
}

func createFolder(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  var userKey *datastore.Key
  if u, err := authorize(c, r, w); err != nil {
    writeError(c, w, err)
    return
  } else {
    userKey = u
  }

  folderName := strings.TrimSpace(r.PostFormValue("folderName"))
  if folderName == "" {
    writeError(c, w, NewReadableError(_l("Missing folder name"), nil))
    return
  }

  if utf8.RuneCountInString(folderName) > 200 {
    writeError(c, w, NewReadableError(_l("Folder name is too long"), nil))
    return
  }

  var subfolders []*SubFolder

  q := datastore.NewQuery("SubFolder").Ancestor(userKey).Filter("Name =", folderName).Limit(1)
  if _, err := q.GetAll(c, &subfolders); err == nil && len(subfolders) > 0 {
    writeError(c, w, NewReadableError(_l("A folder with that name already exists"), nil))
    return
  } else if err != nil && err != datastore.ErrNoSuchEntity {
    writeError(c, w, NewReadableError(_l("An error occurred while searching through existing folders"), &err))
    return
  }

  subfolderKey := datastore.NewKey(c, "SubFolder", "", 0, userKey)
  subfolder := new(SubFolder)

  subfolder.Name = folderName
  if _, err := datastore.Put(c, subfolderKey, subfolder); err != nil {
    writeError(c, w, NewReadableError(_l("An error occurred while adding a new folder"), &err))
    return
  }

  // FIXME
  writeObject(w, map[string]string { "message": _l("FIXME Folder successfully created") })
}

