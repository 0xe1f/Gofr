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
 
package storage

import (
  "appengine"
  "appengine/datastore"
  "errors"
  "strconv"
  "strings"
)

const (
  articlePageSize = 40
)

type UserID string
type Context interface {}

func newUserKey(c appengine.Context, userId UserID) *datastore.Key {
  return datastore.NewKey(c, "User", string(userId), 0, nil)
}

func formatId(kind string, intId int64) string {
  return kind + "://" + strconv.FormatInt(intId, 36)
}

func unformatId(formattedId string) (string, int64, error) {
  if parts := strings.SplitN(formattedId, "://", 2); len(parts) == 2 {
    if id, err := strconv.ParseInt(parts[1], 36, 64); err == nil {
      return parts[0], id, nil
    } else {
      return parts[0], 0, nil
    }
  }

  return "", 0, errors.New("Missing valid identifier")
}

func (filter ArticleFilter)NewQuery(c appengine.Context, start string) (*datastore.Query, error) {
  userKey := newUserKey(c, filter.UserID)

  var ancestorKey *datastore.Key
  if subscriptionID := filter.SubscriptionID; subscriptionID == "" {
    ancestorKey = userKey
  } else if kind, id, err := unformatId(subscriptionID); err != nil {
    return nil, err
  } else {
    if kind == "folder" {
      ancestorKey = datastore.NewKey(c, "Folder", "", id, userKey)
    } else { // Assume it's a subscription ID
      parentKey := userKey
      if folderId := filter.FolderID; folderId != "" {
        if kind, id, err := unformatId(folderId); err == nil {
          if kind == "folder" {
            parentKey = datastore.NewKey(c, "Folder", "", id, userKey)
          }
        } else {
          return nil, err
        }
      }

      ancestorKey = datastore.NewKey(c, "Subscription", subscriptionID, 0, parentKey)
    }
  }

  q := datastore.NewQuery("Article").Ancestor(ancestorKey).Order("-Published")
  if filter.Property != "" {
    q = q.Filter("Properties = ", filter.Property)
  }
  if start != "" {
    if cursor, err := datastore.DecodeCursor(start); err == nil {
      q = q.Start(cursor)
    } else {
      return nil, err
    }
  }

  return q, nil
}

func NewArticlePage(sc Context, filter ArticleFilter, start string) (*ArticlePage, error) {
  c := sc.(appengine.Context)

  var q *datastore.Query
  if query, err := filter.NewQuery(c, start); err != nil {
    return nil, err
  } else {
    q = query
  }

  t := q.Run(c)

  articles := make([]Article, articlePageSize)
  entryKeys := make([]*datastore.Key, articlePageSize)

  var readCount int
  for readCount = 0; readCount < articlePageSize; readCount++ {
    article := &articles[readCount]

    if _, err := t.Next(article); err != nil && err == datastore.Done {
      break
    } else if err != nil {
      return nil, err
    }

    entryKey := article.Entry
    entryKeys[readCount] = entryKey
    article.ID = entryKey.StringID()
    article.Source = entryKey.Parent().StringID()
  }

  continueFrom := ""
  if readCount >= articlePageSize {
    if cursor, err := t.Cursor(); err == nil {
      continueFrom = cursor.String()
    }
  }

  articles = articles[:readCount]
  entryKeys = entryKeys[:readCount]

  entries := make([]Entry, readCount)
  if err := datastore.GetMulti(c, entryKeys, entries); err != nil {
    return nil, err
  }

  for i, _ := range articles {
    articles[i].Details = &entries[i]
  }

  page := ArticlePage {
    Articles: articles,
    Continue: continueFrom,
  }

  return &page, nil
}

func NewUserSubscriptions(sc Context, userID UserID) (*UserSubscriptions, error) {
  c := sc.(appengine.Context)

  var subscriptions []Subscription
  var subscriptionKeys []*datastore.Key

  userKey := newUserKey(c, userID)
  q := datastore.NewQuery("Subscription").Ancestor(userKey).Limit(1000)
  if subKeys, err := q.GetAll(c, &subscriptions); err != nil {
    return nil, err
  } else if subscriptions == nil {
    subscriptions = make([]Subscription, 0)
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
    return nil, err
  }

  for i, _ := range subscriptions {
    subscription := &subscriptions[i]
    feed := feeds[i]
    subscriptionKey := subscriptionKeys[i]

    subscription.ID = subscriptionKey.StringID()
    subscription.Link = feed.Link

    if subscriptionKey.Parent().Kind() == "Folder" {
      subscription.Parent = formatId("folder", subscriptionKey.Parent().IntID())
    }
  }

  var folders []Folder
  var folderKeys []*datastore.Key

  q = datastore.NewQuery("Folder").Ancestor(userKey).Limit(1000)
  if k, err := q.GetAll(c, &folders); err != nil {
    return nil, err
  } else if folders == nil {
    folders = make([]Folder, 0)
  } else {
    folderKeys = k
  }

  for i, _ := range folders {
    folder := &folders[i]
    folder.ID = formatId("folder", folderKeys[i].IntID())
  }

  allItems := Folder {
    ID: "",
    Title: "", // All items
  }

  userSubscriptions := UserSubscriptions {
    Subscriptions: subscriptions,
    Folders: append(folders, allItems),
  }

  return &userSubscriptions, nil
}

func IsFolderTitleDuplicate(sc Context, userID UserID, title string) (bool, error) {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, userID)

  var folders []*Folder
  q := datastore.NewQuery("Folder").Ancestor(userKey).Filter("Title =", title).Limit(1)
  if _, err := q.GetAll(c, &folders); err == nil && len(folders) > 0 {
    return true, nil
  } else if err != nil {
    return false, err
  }

  return false, nil
}

func FolderExists(sc Context, ref FolderRef) (bool, error) {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, ref.UserID)

  if kind, id, err := unformatId(ref.FolderID); err != nil {
    return false, err
  } else if kind == "folder" {
    folderKey := datastore.NewKey(c, "Folder", "", id, userKey)
    folder := new(Folder)

    if err := datastore.Get(c, folderKey, folder); err == nil {
      return true, nil
    } else if err != datastore.ErrNoSuchEntity {
      return false, err
    }
  } else {
    return false, errors.New("Expecting folder ID; found: " + kind)
  }

  return false, nil
}

func CreateFolder(sc Context, userID UserID, title string) error {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, userID)

  folderKey := datastore.NewIncompleteKey(c, "Folder", userKey)
  folder := Folder {
    Title: title,
  }

  if _, err := datastore.Put(c, folderKey, &folder); err != nil {
    return err
  }

  return nil
}

func RenameSubscription(sc Context, ref SubscriptionRef, title string) error {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, ref.UserID)

  ancestorKey := userKey
  if ref.FolderID != "" {
    if kind, id, err := unformatId(ref.FolderID); err != nil {
      return err
    } else if kind == "folder" {
      ancestorKey = datastore.NewKey(c, "Folder", "", id, userKey)
    } else {
      return errors.New("Expecting folder ID; found: " + kind)
    }
  }

  subscriptionKey := datastore.NewKey(c, "Subscription", ref.SubscriptionID, 0, ancestorKey)
  subscription := new(Subscription)

  if err := datastore.Get(c, subscriptionKey, subscription); err != nil {
    return err
  }

  subscription.Title = title
  if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
    return err
  }

  return nil
}

func RenameFolder(sc Context, userID UserID, folderID string, title string) error {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, userID)

  var folderKey *datastore.Key
  if kind, id, err := unformatId(folderID); err != nil {
    return err
  } else if kind == "folder" {
    folderKey = datastore.NewKey(c, "Folder", "", id, userKey)
  } else {
    return errors.New("Expecting folder ID; found: " + kind)
  }

  folder := new(Folder)
  if err := datastore.Get(c, folderKey, folder); err != nil {
    return err
  }

  folder.Title = title
  if _, err := datastore.Put(c, folderKey, folder); err != nil {
    return err
  }

  return nil
}

func SetProperty(sc Context, ref ArticleRef, propertyName string, propertyValue bool) ([]string, error) {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, ref.UserID)

  parentKey := userKey
  if ref.FolderID != "" {
    if kind, id, err := unformatId(ref.FolderID); err != nil {
      return nil, err
    } else if kind == "folder" {
      parentKey = datastore.NewKey(c, "Folder", "", id, userKey)
    } else {
      return nil, errors.New("Expecting folder ID; found: " + kind)
    }
  }

  subscriptionKey := datastore.NewKey(c, "Subscription", ref.SubscriptionID, 0, parentKey)
  articleKey := datastore.NewKey(c, "Article", ref.ArticleID, 0, subscriptionKey)

  article := new(Article)
  if err := datastore.Get(c, articleKey, article); err != nil {
    return nil, err
  }

  // Convert set property list to a map
  propertyMap := make(map[string]bool)
  for _, property := range article.Properties {
    propertyMap[property] = true
  }

  unreadDelta := 0
  writeChanges := false

  // 'read' and 'unread' are mutually exclusive
  if propertyName == "read" {
    if propertyMap[propertyName] && !propertyValue {
      delete(propertyMap, "read")
      propertyMap["unread"] = true
      unreadDelta = 1
    } else if !propertyMap[propertyName] && propertyValue {
      delete(propertyMap, "unread")
      propertyMap["read"] = true
      unreadDelta = -1
    }
    writeChanges = unreadDelta != 0
  } else if propertyName == "unread" {
    if propertyMap[propertyName] && !propertyValue {
      delete(propertyMap, "unread")
      propertyMap["read"] = true
      unreadDelta = -1
    } else if !propertyMap[propertyName] && propertyValue {
      delete(propertyMap, "read")
      propertyMap["unread"] = true
      unreadDelta = 1
    }
    writeChanges = unreadDelta != 0
  } else {
    if propertyMap[propertyName] && !propertyValue {
      delete(propertyMap, propertyName)
      writeChanges = true
    } else if !propertyMap[propertyName] && propertyValue {
      propertyMap[propertyName] = true
      writeChanges = true
    }
  }

  if writeChanges {
    article.Properties = make([]string, len(propertyMap))
    i := 0
    for key, _ := range propertyMap {
      article.Properties[i] = key
      i++
    }

    if _, err := datastore.Put(c, articleKey, article); err != nil {
      return nil, err
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

  return article.Properties, nil
}

func IsFeedAvailable(sc Context, url string) (bool, error) {
  c := sc.(appengine.Context)

  feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
  feed := new(Feed)

  if err := datastore.Get(c, feedKey, feed); err == nil {
    return true, nil
  } else if err != datastore.ErrNoSuchEntity {
    return false, err
  }

  return false, nil
}

func IsSubscribed(sc Context, userID UserID, subscriptionURL string) (bool, error) {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, userID)

  feedKey := datastore.NewKey(c, "Feed", subscriptionURL, 0, nil)
  q := datastore.NewQuery("Subscription").Ancestor(userKey).Filter("Feed =", feedKey).KeysOnly().Limit(1)

  if subKeys, err := q.GetAll(c, nil); err != nil {
    return false, err
  } else if len(subKeys) > 0 {
    return true, nil
  }

  return false, nil
}

func WebToFeedURL(sc Context, url string) (string, error) {
  c := sc.(appengine.Context)

  q := datastore.NewQuery("Feed").Filter("Link =", url).Limit(1)
  var feeds []*Feed
  if _, err := q.GetAll(c, &feeds); err == nil {
    if len(feeds) > 0 {
      return feeds[0].URL, nil
    }
  } else {
    return "", err
  }

  return "", nil
}

/*
func Subscribe(sc Context, ref FolderRef, subscriptionURL string) error {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, ref.UserID)

  if subscriptionURL := strings.TrimSpace(r.PostFormValue("url")); subscriptionURL == "" {
    writeError(c, w, NewReadableError(_l("Missing URL"), nil))
    return
  } else {
    if _, err := url.ParseRequestURI(subscriptionURL); err != nil {
      writeError(c, w, NewReadableError(_l("Subscription URL is not valid"), &err))
      return
    }
    if url := feedURLFromLink(c, subscriptionURL); url != "" {
      feedURL = url
    } else {
      feedURL = subscriptionURL
    }
  }

  if subscriptionKey, err := subscriptionKeyForURL(c, feedURL, userKey); err != nil {
    writeError(c, w, err)
    return
  } else {
    if subscriptionKey != nil {
      writeObject(w, map[string]string { "message": _l(`You are already subscribed to this feed`) })
      return
    }
  }

  folderId := r.PostFormValue("folder")
  if folderId != "" {
    if kind, id, err := unformatId(folderId); err != nil {
      writeError(c, w, err)
      return
    } else if kind == "folder" {
      folder := new(storage.Folder)
      folderKey := datastore.NewKey(c, "Folder", "", id, userKey)

      if err := datastore.Get(c, folderKey, folder); err == datastore.ErrNoSuchEntity {
        writeError(c, w, NewReadableError(_l("Folder not found"), nil))
        return
      } else if err != nil {
        writeError(c, w, err)
        return
      }
    } else {
      writeError(c, w, NewReadableError(_l("Folder is not valid"), nil))
      return
    }
  }

  feedKey := datastore.NewKey(c, "Feed", feedURL, 0, nil)
  feed := new(storage.Feed)

  if err := datastore.Get(c, feedKey, feed); err == nil {
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
    "folderId": { folderId },
    "userID": { user.ID },
  })

  if _, err := taskqueue.Add(c, task, ""); err != nil {
    writeError(c, w, NewReadableError(_l("Cannot subscribe - too busy"), &err))
    return
  }

  writeObject(w, map[string]string { "message": _l("Your subscription has been queued for addition.") })
}
*/