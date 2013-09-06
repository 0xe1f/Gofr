/*****************************************************************************
 **
 ** Gofr
 ** https://github.com/melllvar/Gofr
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
  "rss"
  "errors"
  "time"
)

const (
  articlePageSize = 40
)

type UserID string
type Context interface {}

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

func (ref FolderRef)IsZero() bool {
  return ref.UserID == "" && ref.FolderID == ""
}

func (ref FolderRef)key(c appengine.Context) (*datastore.Key, error) {
  userKey := newUserKey(c, ref.UserID)

  if ref.FolderID != "" {
    if kind, id, err := unformatId(ref.FolderID); err != nil {
      return nil, err
    } else if kind == "folder" {
      return datastore.NewKey(c, "Folder", "", id, userKey), nil
    } else {
      return nil, errors.New("Expecting folder ID; found: " + kind)
    }
  }

  return userKey, nil
}

func (ref SubscriptionRef)key(c appengine.Context) (*datastore.Key, error) {
  userKey := newUserKey(c, ref.UserID)

  ancestorKey := userKey
  if ref.FolderID != "" {
    if kind, id, err := unformatId(ref.FolderID); err != nil {
      return nil, err
    } else if kind == "folder" {
      ancestorKey = datastore.NewKey(c, "Folder", "", id, userKey)
    } else {
      return nil, errors.New("Expecting folder ID; found: " + kind)
    }
  }

  if ref.SubscriptionID == "" {
    return ancestorKey, nil
  }

  return datastore.NewKey(c, "Subscription", ref.SubscriptionID, 0, ancestorKey), nil
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

func IsFolderDuplicate(sc Context, userID UserID, title string) (bool, error) {
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

func IsSubscriptionDuplicate(sc Context, userID UserID, subscriptionURL string) (bool, error) {
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

func FolderByTitle(sc Context, userID UserID, title string) (FolderRef, error) {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, userID)

  q := datastore.NewQuery("Folder").Ancestor(userKey).Filter("Title =", title).KeysOnly().Limit(1)
  if folderKeys, err := q.GetAll(c, nil); err != nil {
    return FolderRef{}, err
  } else if len(folderKeys) > 0 {
    return newFolderRef(userID, folderKeys[0]), nil
  }

  return FolderRef{}, nil
}

func FolderExists(sc Context, ref FolderRef) (bool, error) {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, ref.UserID)

  if ref.FolderID == "" && ref.UserID != "" {
    return true, nil
  }

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

func SubscriptionExists(sc Context, ref SubscriptionRef) (bool, error) {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, ref.UserID)

  parentKey := userKey
  if ref.FolderID != "" {
    if kind, id, err := unformatId(ref.FolderID); err != nil {
      return false, err
    } else if kind == "folder" {
      parentKey = datastore.NewKey(c, "Folder", "", id, userKey)
    } else {
      return false, errors.New("Expecting folder ID; found: " + kind)
    }
  }

  subscriptionKey := datastore.NewKey(c, "Subscription", ref.SubscriptionID, 0, parentKey)
  subscription := new(Subscription)

  if err := datastore.Get(c, subscriptionKey, subscription); err == nil {
    return true, nil
  } else if err != datastore.ErrNoSuchEntity {
    return false, err
  }

  return false, nil
}

func CreateFolder(sc Context, userID UserID, title string) (FolderRef, error) {
  c := sc.(appengine.Context)
  userKey := newUserKey(c, userID)

  folderKey := datastore.NewIncompleteKey(c, "Folder", userKey)
  folder := Folder {
    Title: title,
  }

  if completeKey, err := datastore.Put(c, folderKey, &folder); err != nil {
    return FolderRef{}, err
  } else {
    return newFolderRef(userID, completeKey), nil
  }
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

func MarkAllAsRead(sc Context, ref SubscriptionRef, filter string) error {
  c := sc.(appengine.Context)

  key, err := ref.key(c)
  if err != nil {
    return err
  }

  batchSize := 1000
  entriesRead := 0

  articles := make([]*Article, batchSize)
  articleKeys := make([]*datastore.Key, batchSize)
  
  q := datastore.NewQuery("Article").Ancestor(key).Filter("Properties =", "unread")
  for t := q.Run(c); ; entriesRead++ {
    article := new(Article)
    articleKey, err := t.Next(article)

    if err == datastore.Done || entriesRead + 1 >= batchSize {
      // Write the batch
      if entriesRead > 0 {
        if _, err := datastore.PutMulti(c, articleKeys[:entriesRead], articles[:entriesRead]); err != nil {
          c.Errorf("Error writing Article batch: %s", err)
          return err
        }
      }

      entriesRead = 0

      if err == datastore.Done {
        break
      }
    } else if err != nil {
      return err
    }

    propertyMap := make(map[string]bool)
    for _, property := range article.Properties {
      propertyMap[property] = true
    }

    delete(propertyMap, "unread")
    propertyMap["read"] = true

    article.Properties = make([]string, len(propertyMap))
    i := 0
    for key, _ := range propertyMap {
      article.Properties[i] = key
      i++
    }

    articleKeys[entriesRead] = articleKey
    articles[entriesRead] = article
  }

  // Reset unread counters
  if key.Kind() != "Subscription" {
    var subscriptions []*Subscription
    q = datastore.NewQuery("Subscription").Ancestor(key).Limit(1000)
    
    if subscriptionKeys, err := q.GetAll(c, &subscriptions); err != nil {
      return err
    } else {
      for _, subscription := range subscriptions {
        subscription.UnreadCount = 0
      }

      if _, err := datastore.PutMulti(c, subscriptionKeys, subscriptions); err != nil {
        return err
      }
    }
  } else {
    subscription := new(Subscription)
    if err := datastore.Get(c, key, subscription); err != nil {
      return err
    }

    subscription.UnreadCount = 0
    if _, err := datastore.Put(c, key, subscription); err != nil {
      return err
    }
  }

  return nil
}

func FeedByURL(sc Context, url string) (*Feed, error) {
  c := sc.(appengine.Context)

  feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
  feed := new(Feed)

  if err := datastore.Get(c, feedKey, feed); err == nil {
    return feed, nil
  } else if err != datastore.ErrNoSuchEntity {
    return nil, err
  }

  return nil, nil
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

func Subscribe(sc Context, ref FolderRef, url string, title string) (SubscriptionRef, error) {
  c := sc.(appengine.Context)

  folderKey, err := ref.key(c)
  if err != nil {
    return SubscriptionRef{}, err
  }

  subscription := new(Subscription)
  subscriptionKey := datastore.NewKey(c, "Subscription", url, 0, folderKey)

  if err := datastore.Get(c, subscriptionKey, subscription); err == nil {
    return SubscriptionRef{
      FolderRef: ref,
      SubscriptionID: url,
    }, nil // Already subscribed
  } else if err == datastore.ErrNoSuchEntity {
    subscription.Updated = time.Time {}
    subscription.Subscribed = time.Now()
    subscription.Title = title
    subscription.UnreadCount = 0
    subscription.Feed = datastore.NewKey(c, "Feed", url, 0, nil)
  } else {
    return SubscriptionRef{}, err
  }

  if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
    return SubscriptionRef{}, err
  }

  return SubscriptionRef{
    FolderRef: ref,
    SubscriptionID: url,
  }, nil
}

func Unsubscribe(sc Context, ref SubscriptionRef) error {
  c := sc.(appengine.Context)

  if key, err := ref.key(c); err != nil {
    return err
  } else {
    return unsubscribe(c, key)
  }
}

func UnsubscribeAllInFolder(sc Context, ref FolderRef) error {
  c := sc.(appengine.Context)
  
  if key, err := ref.key(c); err != nil {
    return err
  } else {
    return unsubscribe(c, key)
  }
}

func UpdateFeed(sc Context, parsedFeed *rss.Feed) error {
  c := sc.(appengine.Context)

  feed, err := NewFeed(parsedFeed)
  if err != nil {
    return err
  }

  feedKey := datastore.NewKey(c, "Feed", feed.URL, 0, nil)

  batchSize := 1000
  elements := len(feed.Entries)

  entryKeys := make([]*datastore.Key, batchSize)
  entryMetaKeys := make([]*datastore.Key, batchSize)

  pending := 0
  written := 0

  for i := 0; ; i++ {
    if i >= elements || pending + 1 >= batchSize {
      if pending > 0 {
        if _, err := datastore.PutMulti(c, entryKeys[:pending], feed.Entries[written:written + pending]); err != nil {
          c.Errorf("Error writing Entry batch: %s", err)
          return err
        }
        if _, err := datastore.PutMulti(c, entryMetaKeys[:pending], feed.EntryMetas[written:written + pending]); err != nil {
          c.Errorf("Error writing EntryMetas batch: %s", err)
          return err
        }
      }

      written += pending
      pending = 0

      if i >= elements {
        break
      }
    }

    keyName := feed.Entries[i].UniqueID
    if keyName == "" {
      c.Warningf("UniqueID for an entry (titled '%s') is missing", feed.Entries[i].Title)
      continue
    }

    entryKeys[i] = datastore.NewKey(c, "Entry", keyName, 0, feedKey)
    entryMetaKeys[i] = datastore.NewKey(c, "EntryMeta", keyName, 0, feedKey)
    feed.EntryMetas[i].Entry = entryKeys[i]

    pending++
  }

  if _, err := datastore.Put(c, feedKey, feed); err != nil {
    c.Errorf("Error writing feed: %s", err)
    return err
  }

  return nil
}

func UpdateSubscription(sc Context, url string, ref SubscriptionRef) error {
  c := sc.(appengine.Context)

  batchSize := 1000
  mostRecentEntryTime := time.Time {}
  articles := make([]Article, batchSize)
  articleKeys := make([]*datastore.Key, batchSize)

  pending, written := 0, 0

  subscriptionKey, err := ref.key(c)
  if err != nil {
    return err
  }

  subscription := new(Subscription)
  if err := datastore.Get(c, subscriptionKey, subscription); err != nil {
    c.Errorf("Error getting subscription: %s", err)
    return err
  }

  feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
  q := datastore.NewQuery("EntryMeta").Ancestor(feedKey).Filter("Retrieved >", subscription.Updated)
  for t := q.Run(c); ; pending++ {
    entryMeta := new(EntryMeta)
    _, err := t.Next(entryMeta)

    if err == datastore.Done || pending + 1 >= batchSize {
      // Write the batch
      if pending > 0 {
        if _, err := datastore.PutMulti(c, articleKeys[:pending], articles[:pending]); err != nil {
          c.Errorf("Error writing Article batch: %s", err)
          return err
        }
      }

      written += pending
      pending = 0

      if err == datastore.Done {
        break
      }
    } else if err != nil {
      c.Errorf("Error reading Entry: %s", err)
      return err
    }

    articleKeys[pending] = datastore.NewKey(c, "Article", entryMeta.Entry.StringID(), 0, subscriptionKey)
    articles[pending].Entry = entryMeta.Entry
    articles[pending].Retrieved = entryMeta.Retrieved
    articles[pending].Published = entryMeta.Published
    articles[pending].Properties = []string { "unread" }

    mostRecentEntryTime = entryMeta.Retrieved
  }

  // Write the subscription
  subscription.Updated = mostRecentEntryTime
  subscription.UnreadCount += written

  if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
    c.Errorf("Error writing subscription: %s", err)
    return err
  }

  return nil
}
