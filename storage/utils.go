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
  "errors"
  "strconv"
  "strings"
  "time"
)

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

func newFolderRef(userID UserID, key *datastore.Key) (FolderRef) {
  ref := FolderRef {
    UserID: userID,
  }

  if key != nil {
    ref.FolderID = formatId("folder", key.IntID())
  }

  return ref
}

func unsubscribe(c appengine.Context, ancestorKey *datastore.Key) error {
  batchWriter := NewBatchWriter(c, BatchDelete)

  q := datastore.NewQuery("Article").Ancestor(ancestorKey).KeysOnly()
  for t := q.Run(c); ; {
    articleKey, err := t.Next(nil)

    if err == datastore.Done {
      break
    } else if err != nil {
      return err
    }

    if err := batchWriter.EnqueueKey(articleKey); err != nil {
      c.Errorf("Error queueing article for batch delete: %s", err)
      return err
    }
  }

  if err := batchWriter.Flush(); err != nil {
    c.Errorf("Error flushing batch queue: %s", err)
    return err
  }

  if ancestorKey.Kind() == "Folder" {
    // Remove subscriptions under the folder
    q = datastore.NewQuery("Subscription").Ancestor(ancestorKey).KeysOnly().Limit(400)
    if subscriptionKeys, err := q.GetAll(c, nil); err == nil {
      if err := datastore.DeleteMulti(c, subscriptionKeys); err != nil {
        return err
      }
    }
  }

  if err := datastore.Delete(c, ancestorKey); err != nil {
    return err
  }

  return nil
}

func updateSubscriptionByKey(c appengine.Context, subscriptionKey *datastore.Key, subscription Subscription) error {
  unreadDelta := 0
  feedKey := subscription.Feed

  // Update usage index (rough way to track feed popularity)
  // No sharding, no transactions - complete accuracy is unimportant for now

  feedUsage := FeedUsage{}
  feedUsageKey := datastore.NewKey(c, "FeedUsage", feedKey.StringID(), 0, nil)

  if err := datastore.Get(c, feedUsageKey, &feedUsage); err == datastore.ErrNoSuchEntity || err == nil {
    if err == datastore.ErrNoSuchEntity {
      // Create a new entity
      feedUsage.Feed = feedKey
    }

    feedUsage.UpdateCount++
    feedUsage.LastSubscriptionUpdate = time.Now()

    if _, err := datastore.Put(c, feedUsageKey, &feedUsage); err != nil {
      c.Warningf("Non-critical error updating feed usage (%s): %s", feedKey.StringID(), err)
    }
  }

  // Find the largest update index for the subscription
  maxUpdateIndex := int64(-1)
  articles := make([]Article, 1)

  q := datastore.NewQuery("Article").Ancestor(subscriptionKey).Order("-UpdateIndex").Limit(1)
  if _, err := q.GetAll(c, &articles); err != nil {
    return err
  } else if len(articles) > 0 {
    maxUpdateIndex = articles[0].UpdateIndex
  }

  batchWriter := NewBatchWriter(c, BatchPut)

  q = datastore.NewQuery("EntryMeta").Ancestor(feedKey).Filter("UpdateIndex >", maxUpdateIndex)
  for t := q.Run(c); ; {
    entryMeta := new(EntryMeta)
    _, err := t.Next(entryMeta)

    if err == datastore.Done {
      break
    } else if err != nil {
      c.Errorf("Error reading Entry: %s", err)
      return err
    }

    articleKey := datastore.NewKey(c, "Article", entryMeta.Entry.StringID(), 0, subscriptionKey)
    article := Article{}

    if err := datastore.Get(c, articleKey, &article); err == datastore.ErrNoSuchEntity {
      // New article
      article.Entry = entryMeta.Entry
      article.Properties = []string { "unread" }
      unreadDelta++
    } else if err != nil {
      c.Warningf("Error reading article %s: %s", entryMeta.Entry.StringID(), err)
      continue
    }

    article.UpdateIndex = entryMeta.UpdateIndex
    article.Fetched = entryMeta.Fetched
    article.Published = entryMeta.Published

    if err := batchWriter.Enqueue(articleKey, &article); err != nil {
      c.Errorf("Error queueing article for batch write: %s", err)
      return err
    }
  }

  if err := batchWriter.Flush(); err != nil {
    c.Errorf("Error flushing batch queue: %s", err)
    return err
  }

  // Write the subscription
  subscription.Updated = time.Now()
  subscription.UnreadCount += unreadDelta

  if _, err := datastore.Put(c, subscriptionKey, &subscription); err != nil {
    c.Errorf("Error writing subscription: %s", err)
    return err
  }

  return nil
}

func updateSubscriptionAsync(c appengine.Context, subscriptionKey *datastore.Key, subscription Subscription, ch chan<- Subscription) {
  err := updateSubscriptionByKey(c, subscriptionKey, subscription)
  if err != nil {
    c.Errorf("Error updating subscription %s: %s", subscription.Title, err)
  }

  ch <- subscription
}