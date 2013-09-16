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
  "html"
  "rss"
  "errors"
  "time"
)

const (
  articlePageSize = 40
)

func (ref FolderRef)IsZero() bool {
  return ref.UserID == "" && ref.FolderID == ""
}

func (userID UserID)key(c appengine.Context) (*datastore.Key, error) {
  if userID == "" {
    return nil, errors.New("UserID is empty")
  }

  return datastore.NewKey(c, "User", string(userID), 0, nil), nil
}

func (ref FolderRef)key(c appengine.Context) (*datastore.Key, error) {
  userKey, err := ref.UserID.key(c)
  if err != nil {
    return nil, err
  }

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
  ancestorKey, err := ref.FolderRef.key(c)
  if err != nil {
    return nil, err
  }

  if ref.SubscriptionID == "" {
    return nil, errors.New("SubscriptionRef is missing Subscription ID")
  }

  return datastore.NewKey(c, "Subscription", ref.SubscriptionID, 0, ancestorKey), nil
}

func (scope ArticleScope)key(c appengine.Context) (*datastore.Key, error) {
  ancestorKey, err := scope.FolderRef.key(c)
  if err != nil {
    return nil, err
  }

  if scope.SubscriptionID == "" {
    return ancestorKey, nil
  }

  return datastore.NewKey(c, "Subscription", scope.SubscriptionID, 0, ancestorKey), nil
}

func (ref ArticleRef)key(c appengine.Context) (*datastore.Key, error) {
  if ref.SubscriptionRef.SubscriptionID == "" {
    return nil, errors.New("Article reference is missing subscription ID")
  }

  subscriptionKey, err := ref.SubscriptionRef.key(c)
  if err != nil {
    return nil, err
  }

  return datastore.NewKey(c, "Article", ref.ArticleID, 0, subscriptionKey), nil
}

func (user User)key(c appengine.Context) (*datastore.Key, error) {
  if user.ID == "" {
    return nil, errors.New("User missing an ID")
  }

  return datastore.NewKey(c, "User", user.ID, 0, nil), nil
}

func NewArticlePage(c appengine.Context, filter ArticleFilter, start string) (*ArticlePage, error) {
  scopeKey, err := filter.key(c)
  if err != nil {
    return nil, err
  }

  q := datastore.NewQuery("Article").Ancestor(scopeKey).Order("-Published")
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

func NewUserSubscriptions(c appengine.Context, userID UserID) (*UserSubscriptions, error) {
  var subscriptions []Subscription
  var subscriptionKeys []*datastore.Key

  userKey, err := userID.key(c)
  if err != nil {
    return nil, err
  }

  q := datastore.NewQuery("Subscription").Ancestor(userKey).Limit(400)
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

  q = datastore.NewQuery("Folder").Ancestor(userKey).Limit(400)
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

func IsFolderDuplicate(c appengine.Context, userID UserID, title string) (bool, error) {
  userKey, err := userID.key(c)
  if err != nil {
    return false, err
  }

  var folders []*Folder
  q := datastore.NewQuery("Folder").Ancestor(userKey).Filter("Title =", title).Limit(1)
  if _, err := q.GetAll(c, &folders); err == nil && len(folders) > 0 {
    return true, nil
  } else if err != nil {
    return false, err
  }

  return false, nil
}

func IsSubscriptionDuplicate(c appengine.Context, userID UserID, subscriptionURL string) (bool, error) {
  userKey, err := userID.key(c)
  if err != nil {
    return false, err
  }

  feedKey := datastore.NewKey(c, "Feed", subscriptionURL, 0, nil)
  q := datastore.NewQuery("Subscription").Ancestor(userKey).Filter("Feed =", feedKey).KeysOnly().Limit(1)

  if subKeys, err := q.GetAll(c, nil); err != nil {
    return false, err
  } else if len(subKeys) > 0 {
    return true, nil
  }

  return false, nil
}

func UserByID(c appengine.Context, userID string) (*User, error) {
  userKey := datastore.NewKey(c, "User", userID, 0, nil)
  user := User{}

  if err := datastore.Get(c, userKey, &user); err == nil {
    return &user, nil
  } else if err != datastore.ErrNoSuchEntity {
    return nil, err
  }

  return nil, nil
}

func (user User)Save(c appengine.Context) error {
  userKey, err := UserID(user.ID).key(c)
  if err != nil {
    return err
  }

  if _, err := datastore.Put(c, userKey, &user); err != nil {
    return err
  }

  return nil
}

func FolderByTitle(c appengine.Context, userID UserID, title string) (FolderRef, error) {
  userKey, err := userID.key(c)
  if err != nil {
    return FolderRef{}, err
  }
  
  q := datastore.NewQuery("Folder").Ancestor(userKey).Filter("Title =", title).KeysOnly().Limit(1)
  if folderKeys, err := q.GetAll(c, nil); err != nil {
    return FolderRef{}, err
  } else if len(folderKeys) > 0 {
    return newFolderRef(userID, folderKeys[0]), nil
  }

  return FolderRef{}, nil
}

func FolderExists(c appengine.Context, ref FolderRef) (bool, error) {
  if folderKey, err := ref.key(c); err != nil {
    return false, err
  } else {
    folder := new(Folder)
    if err := datastore.Get(c, folderKey, folder); err == nil {
      return true, nil
    } else if err != datastore.ErrNoSuchEntity {
      return false, err
    }
  }

  return false, nil
}

func SubscriptionExists(c appengine.Context, ref SubscriptionRef) (bool, error) {
  if subscriptionKey, err := ref.key(c); err != nil {
    return false, err
  } else {
    subscription := new(Subscription)
    if err := datastore.Get(c, subscriptionKey, subscription); err == nil {
      return true, nil
    } else if err != datastore.ErrNoSuchEntity {
      return false, err
    }
  }

  return false, nil
}

func CreateFolder(c appengine.Context, userID UserID, title string) (FolderRef, error) {
  userKey, err := userID.key(c)
  if err != nil {
    return FolderRef{}, err
  }
  
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

func RenameSubscription(c appengine.Context, ref SubscriptionRef, title string) error {
  subscriptionKey, err := ref.key(c)
  if err != nil {
    return err
  }

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

func RenameFolder(c appengine.Context, ref FolderRef, title string) error {
  folderKey, err := ref.key(c)
  if err != nil {
    return err
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

func SetProperty(c appengine.Context, ref ArticleRef, propertyName string, propertyValue bool) ([]string, error) {
  articleKey, err := ref.key(c)
  if err != nil {
    return nil, err
  }

  article := new(Article)
  if err := datastore.Get(c, articleKey, article); err != nil {
    return nil, err
  }

  if propertyValue != article.HasProperty(propertyName) {
    wasUnread := article.IsUnread()
    unreadDelta := 0

    article.SetProperty(propertyName, propertyValue)

    // Update unread counts if necessary
    if wasUnread != article.IsUnread() {
      if wasUnread {
        unreadDelta = -1
      } else {
        unreadDelta = 1
      }
    }

    if _, err := datastore.Put(c, articleKey, article); err != nil {
      return nil, err
    }

    if unreadDelta != 0 {
      // Update unread counts - not critical
      subscriptionKey := articleKey.Parent()
      subscription := new(Subscription)

      if err := datastore.Get(c, subscriptionKey, subscription); err != nil {
        c.Warningf("Unread count update failed: subscription read error (%s)", err)
      } else {
        subscription.UnreadCount += unreadDelta
        if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
          c.Warningf("Unread count update failed: subscription write error (%s)", err)
        }
      }
    }
  }

  return article.Properties, nil
}

func MarkAllAsRead(c appengine.Context, scope ArticleScope) (int, error) {
  key, err := scope.key(c)
  if err != nil {
    return 0, err
  }

  batchWriter := NewBatchWriter(c, BatchPut)

  q := datastore.NewQuery("Article").Ancestor(key).Filter("Properties =", "unread")
  for t := q.Run(c); ; {
    article := new(Article)
    articleKey, err := t.Next(article)

    if err == datastore.Done {
      break
    } else if err != nil {
      c.Errorf("Error reading Article: %s", err)
      return 0, err
    }

    article.SetProperty("read", true)

    if err := batchWriter.Enqueue(articleKey, article); err != nil {
      c.Errorf("Error queueing article for batch write: %s", err)
      return 0, err
    }
  }

  if err := batchWriter.Flush(); err != nil {
    c.Errorf("Error flushing batch queue: %s", err)
    return 0, err
  }

  // Reset unread counters
  if key.Kind() != "Subscription" {
    var subscriptions []*Subscription
    q = datastore.NewQuery("Subscription").Ancestor(key).Limit(400)
    
    if subscriptionKeys, err := q.GetAll(c, &subscriptions); err != nil {
      return 0, err
    } else {
      for _, subscription := range subscriptions {
        subscription.UnreadCount = 0
      }

      if _, err := datastore.PutMulti(c, subscriptionKeys, subscriptions); err != nil {
        return 0, err
      }
    }
  } else {
    subscription := new(Subscription)
    if err := datastore.Get(c, key, subscription); err != nil {
      return 0, err
    }

    subscription.UnreadCount = 0
    if _, err := datastore.Put(c, key, subscription); err != nil {
      return 0, err
    }
  }

  return batchWriter.Written(), nil
}

func FeedByURL(c appengine.Context, url string) (*Feed, error) {
  feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
  feed := new(Feed)

  if err := datastore.Get(c, feedKey, feed); err == nil {
    return feed, nil
  } else if err != datastore.ErrNoSuchEntity {
    return nil, err
  }

  return nil, nil
}

func IsFeedAvailable(c appengine.Context, url string) (bool, error) {
  feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
  feed := new(Feed)

  if err := datastore.Get(c, feedKey, feed); err == nil {
    return true, nil
  } else if err != datastore.ErrNoSuchEntity {
    return false, err
  }

  return false, nil
}

func WebToFeedURL(c appengine.Context, url string) (string, error) {
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

func Subscribe(c appengine.Context, ref FolderRef, url string, title string) (SubscriptionRef, error) {
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
    subscription.MaxUpdateIndex = -1
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

func Unsubscribe(c appengine.Context, ref SubscriptionRef) error {
  if key, err := ref.key(c); err != nil {
    return err
  } else {
    return unsubscribe(c, key)
  }
}

func UnsubscribeAllInFolder(c appengine.Context, ref FolderRef) error {
  if key, err := ref.key(c); err != nil {
    return err
  } else {
    return unsubscribe(c, key)
  }
}

func UpdateFeed(c appengine.Context, parsedFeed *rss.Feed) error {
  var updateCounter int64
  
  feed := Feed{}
  feedKey := datastore.NewKey(c, "Feed", parsedFeed.URL, 0, nil)
  var lastFetched time.Time

  err := datastore.RunInTransaction(c, func(c appengine.Context) error {
    if err := datastore.Get(c, feedKey, &feed); err == datastore.ErrNoSuchEntity {
      // New; set defaults
      feed.URL = parsedFeed.URL
      feed.UpdateCounter = 0
    } else if err != nil {
      // Some other error
      return err
    }

    durationBetweenUpdates := parsedFeed.DurationBetweenUpdates()

    lastFetched = feed.Fetched

    feed.Title = parsedFeed.Title
    feed.Description = parsedFeed.Description
    feed.Updated = parsedFeed.Updated
    feed.Link = parsedFeed.WWWURL
    feed.Format = parsedFeed.Format
    feed.Fetched = parsedFeed.Retrieved
    feed.NextFetch = parsedFeed.Retrieved.Add(durationBetweenUpdates)
    feed.HourlyUpdateFrequency = float32(durationBetweenUpdates.Hours())

    // Increment update counter
    feed.UpdateCounter += int64(len(parsedFeed.Entries))
    updateCounter = feed.UpdateCounter

    if updatedKey, err := datastore.Put(c, feedKey, &feed); err != nil {
      return err
    } else {
      feedKey = updatedKey
    }

    return nil
  }, nil)

  if err != nil {
    c.Errorf("Error incrementing entry counter: %s", err)
    return err
  }

  batchSize := 400
  elements := len(parsedFeed.Entries)

  entryKeys := make([]*datastore.Key, batchSize)
  entries := make([]*Entry, batchSize)

  entryMetaKeys := make([]*datastore.Key, batchSize)
  entryMetas := make([]*EntryMeta, batchSize)

  pending := 0
  written := 0

  started := time.Now()
  nuovo, unchanged, changed := 0, 0, 0

  for i := 0; ; i++ {
    if i >= elements || pending + 1 >= batchSize {
      if pending > 0 {
        if _, err := datastore.PutMulti(c, entryKeys[:pending], entries[:pending]); err != nil {
          if multiError, ok := err.(appengine.MultiError); ok {
            for i, err := range multiError {
              if err != nil {
                c.Errorf("entry[%d]: key [%s] failed: %s", i, 
                  entryKeys[i].StringID(), err)
              }
            }
          }
          // FIXME: don't stop the entire write simply because a few entries failed
          return err
        }
        if _, err := datastore.PutMulti(c, entryMetaKeys[:pending], entryMetas[:pending]); err != nil {
          if multiError, ok := err.(appengine.MultiError); ok {
            for i, err := range multiError {
              if err != nil {
                c.Errorf("entryMeta[%d]: [%s] failed: %s", i, 
                  entryMetaKeys[i].StringID(), err)
              }
            }
          }
          // FIXME: don't stop the entire write simply because a few entries failed
          return err
        }
      }

      written += pending
      pending = 0

      if i >= elements {
        break
      }
    }

    parsedEntry := parsedFeed.Entries[i]
    entryGUID := parsedEntry.UniqueID()
    if entryGUID == "" {
      c.Warningf("Missing GUID for an entry titled '%s'", parsedEntry.Title)
      continue
    }

    entryMetaKey := datastore.NewKey(c, "EntryMeta", entryGUID, 0, feedKey)
    entryKey := datastore.NewKey(c, "Entry", entryGUID, 0, feedKey)
    var entryMeta EntryMeta

    if err := datastore.Get(c, entryMetaKey, &entryMeta); err == datastore.ErrNoSuchEntity {
      // New; set defaults
      entryMeta.Entry = entryKey
      nuovo++
    } else if err != nil {
      // Some other error
      c.Warningf("Error getting entry meta (GUID '%s'): err", entryGUID, err)
      continue
    } else {
      // Already in the store - check if it's new/updated
      if !entryMeta.Updated.IsZero() && entryMeta.Updated.Equal(parsedEntry.Updated) {
        // No updates - skip
        unchanged++
        continue
      } else {
        changed++
      }
    }

    entryMeta.Published = parsedEntry.Published
    entryMeta.Updated = parsedEntry.Updated
    entryMeta.Fetched = parsedFeed.Retrieved
    entryMeta.UpdateIndex = updateCounter

    // At this point, metadata tells us the record needs updating, so we 
    // just overwrite everything under the entry

    entry := Entry {
      UniqueID: entryGUID,
      Author: html.UnescapeString(parsedEntry.Author),
      Title: html.UnescapeString(parsedEntry.Title),
      Link: parsedEntry.WWWURL,
      Summary: parsedEntry.GenerateSummary(),
      Content: parsedEntry.Content,

      // FIXME: Get rid of this eventually - already part of meta
      Published: parsedEntry.Published,
      Updated: parsedEntry.Updated,
    }

    entryMetas[pending] = &entryMeta
    entryMetaKeys[pending] = entryMetaKey
    entries[pending] = &entry
    entryKeys[pending] = entryKey

    updateCounter++
    pending++
  }

  if appengine.IsDevAppServer() {
    c.Debugf("Completed %s: %d,%d,%d (n,c,u) (took %s, last fetch: %s ago)", 
      parsedFeed.URL, nuovo, changed, unchanged, time.Since(started), time.Since(lastFetched))
  }

  return nil
}

func UpdateSubscription(c appengine.Context, url string, ref SubscriptionRef) (int, error) {
  subscriptionKey, err := ref.key(c)
  if err != nil {
    return 0, err
  }

  subscription := Subscription{}
  if err := datastore.Get(c, subscriptionKey, &subscription); err != nil {
    c.Errorf("Error getting subscription: %s", err)
    return 0, err
  }

  return updateSubscriptionByKey(c, subscriptionKey, subscription)
}

func UpdateAllSubscriptions(c appengine.Context, userID UserID) error {
  userKey, err := userID.key(c)
  if err != nil {
    return err
  }
  
  var subscriptions []Subscription

  q := datastore.NewQuery("Subscription").Ancestor(userKey).Limit(400)
  subscriptionKeys, err := q.GetAll(c, &subscriptions)
  if err != nil {
    return err
  }

  started := time.Now()
  doneChannel := make(chan Subscription)
  subscriptionCount := len(subscriptions)

  for i := 0; i < subscriptionCount; i++ {
    go updateSubscriptionAsync(c, subscriptionKeys[i], subscriptions[i], doneChannel)
  }

  for i := 0; i < subscriptionCount; i++ {
    <-doneChannel;
  }

  c.Infof("%d subscriptions completed in %s", subscriptionCount, time.Since(started))

  return nil
}

func AreNewEntriesAvailable(c appengine.Context, subscriptions []Subscription) (bool, error) {
  for _, subscription := range subscriptions {
    q := datastore.NewQuery("EntryMeta").Ancestor(subscription.Feed).Filter("UpdateIndex >", subscription.MaxUpdateIndex).KeysOnly().Limit(1)
    if entryMetaKeys, err := q.GetAll(c, nil); err != nil {
      return false, err
    } else if len(entryMetaKeys) > 0 {
      return true, nil
    }
  }

  return false, nil
}

func UpdateUnreadCounts(c appengine.Context, ch chan<- Subscription, subscriptionKey *datastore.Key, subscription Subscription) {
  originalSubscriptionCount := subscription.UnreadCount
  q := datastore.NewQuery("Article").Ancestor(subscriptionKey).Filter("Properties =", "unread")

  if count, err := q.Count(c); err != nil {
    c.Errorf("Error getting unread count: %s", err)
    goto done
  } else if count != originalSubscriptionCount {
    subscription.UnreadCount = count
    if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
      c.Errorf("Error writing unread count: %s", err)
      goto done
    }
  }

  if originalSubscriptionCount != subscription.UnreadCount {
    c.Infof("Subscription count corrected to %d (was: %d)", 
      subscription.UnreadCount, originalSubscriptionCount)
  }

done:
  if ch != nil {
    ch<- subscription
  }
}

