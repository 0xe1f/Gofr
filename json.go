/*****************************************************************************
 **
 ** FRAE
 ** https://github.com/melllvar/frae
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
 
package frae

import (
  "appengine"
  "appengine/datastore"
  "appengine/taskqueue"
  "appengine/urlfetch"
  "appengine/user"
  "encoding/json"
  "net/url"
  "net/http"
  "fmt"
  "parser"
  "strings"
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

func getEntries(c appengine.Context, ancestorKey *datastore.Key, filterProperty string) ([]SubEntry, error) {
  var subEntries []SubEntry

  q := datastore.NewQuery("SubEntry").Ancestor(ancestorKey).Order("-Published").Limit(40)
  if filterProperty != "" {
    q = q.Filter("Properties = ", filterProperty)
  }

  if _, err := q.GetAll(c, &subEntries); err != nil {
    return nil, err
  } else {
    entries := make([]Entry, len(subEntries))

    entryKeys := make([]*datastore.Key, len(subEntries))
    for i, subEntry := range subEntries {
      entryKeys[i] = subEntry.Entry
    }

    if err := datastore.GetMulti(c, entryKeys, entries); err != nil {
      return nil, err
    }

    for i, _ := range subEntries {
      subEntries[i].ID = entryKeys[i].StringID()
      subEntries[i].Source = entryKeys[i].Parent().StringID()
      subEntries[i].Details = &entries[i]
    }
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

  filterProperty := r.FormValue("filter")
  if !validProperties[filterProperty] {
    filterProperty = ""
  }

  if entries, err := getEntries(c, ancestorKey, filterProperty); err != nil {
    writeError(c, w, err)
    return
  } else {
    w.Header().Set("Content-type", "application/json; charset=utf-8")
    
    writeObject(w, entries)
  }
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

  subscriptionUrl := strings.TrimSpace(r.PostFormValue("url"))

  if subscriptionUrl == "" {
    writeError(c, w, NewReadableError(_l("Missing URL"), nil))
    return
  }

  subscriptionKey := datastore.NewKey(c, "Subscription", subscriptionUrl, 0, userKey)
  subscription := new(Subscription)

  if err := datastore.Get(c, subscriptionKey, subscription); err == nil {
    writeObject(w, map[string]string { "message": _l("You are already subscribed to %s", subscription.Title) })
    return
  } else if err != datastore.ErrNoSuchEntity {
    writeError(c, w, err)
    return
  }

  feedKey := datastore.NewKey(c, "Feed", subscriptionUrl, 0, nil)
  if err := datastore.Get(c, feedKey, nil); err == nil {
    // Already have the feed
  } else if err == datastore.ErrNoSuchEntity {
    // Don't have the feed - fetch it
    client := urlfetch.Client(c)
    if response, err := client.Get(subscriptionUrl); err != nil {
      writeError(c, w, NewReadableError(_l("An error occurred while downloading the feed"), &err))
      return
    } else {
      defer response.Body.Close()
      if _, err := parser.UnmarshalStream(subscriptionUrl, response.Body); err != nil {
        writeError(c, w, NewReadableError(_l("An error occurred while parsing the feed"), &err))
        return
      }
    }
  } else {
    // Some other error
    writeError(c, w, err)
    return
  }

  user := user.Current(c)
  task := taskqueue.NewPOSTTask("/tasks/subscribe", url.Values {
    "url": { subscriptionUrl },
    "userID": { user.ID },
  })
  task.Name = "subscribe " + user.ID + "@" + subscriptionUrl

  if _, err := taskqueue.Add(c, task, ""); err != nil {
    writeError(c, w, NewReadableError(_l("Subscription may already have been queued"), &err))
    return
  }

  writeObject(w, map[string]string { "message": _l("Your subscription has been queued") })
}
