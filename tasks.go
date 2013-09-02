/*****************************************************************************
 **
 ** PerFeediem
 ** https://github.com/melllvar/PerFeediem
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
  "appengine/urlfetch"
  "net/http"
  "opml"
  "rss"
  "storage"
  "time"
)

func registerTasks() {
  http.HandleFunc("/tasks/subscribe", subscribeTask)
  http.HandleFunc("/tasks/unsubscribe", unsubscribeTask)
  http.HandleFunc("/tasks/import", importOPMLTask)
  http.HandleFunc("/tasks/markAllAsRead", markAllAsReadTask)
}

func updateSubscription(c appengine.Context, subscriptionKey *datastore.Key, feedKey *datastore.Key, feed *storage.Feed) error {
  batchSize := 1000
  mostRecentEntryTime := time.Time {}
  articles := make([]storage.Article, batchSize)
  articleKeys := make([]*datastore.Key, batchSize)
  entriesRead := 0
  entriesWritten := 0

  subscription := new(storage.Subscription)
  if err := datastore.Get(c, subscriptionKey, subscription); err != nil && err != datastore.ErrNoSuchEntity {
    c.Errorf("Error getting subscription: %s", err)
    return err
  } else if err == datastore.ErrNoSuchEntity {
    // Set defaults
    subscription.Title = feed.Title
    subscription.Subscribed = time.Now().UTC()
    subscription.Feed = feedKey
  }

  q := datastore.NewQuery("EntryMeta").Ancestor(feedKey).Filter("Retrieved >", subscription.Updated)
  for t := q.Run(c); ; entriesRead++ {
    entryMeta := new(storage.EntryMeta)
    _, err := t.Next(entryMeta)

    if err == datastore.Done || entriesRead + 1 >= batchSize {
      // Write the batch
      if entriesRead > 0 {
        if _, err := datastore.PutMulti(c, articleKeys[:entriesRead], articles[:entriesRead]); err != nil {
          c.Errorf("Error writing Article batch: %s", err)
          return err
        }
      }

      entriesWritten += entriesRead
      entriesRead = 0

      if err == datastore.Done {
        break
      }
    } else if err != nil {
      c.Errorf("Error reading Entry: %s", err)
      return err
    }

    articleKeys[entriesRead] = datastore.NewKey(c, "Article", entryMeta.Entry.StringID(), 0, subscriptionKey)
    articles[entriesRead].Entry = entryMeta.Entry
    articles[entriesRead].Retrieved = entryMeta.Retrieved
    articles[entriesRead].Published = entryMeta.Published
    articles[entriesRead].Properties = []string { "unread" }

    mostRecentEntryTime = entryMeta.Retrieved
  }

  // Write the subscription
  subscription.Updated = mostRecentEntryTime
  subscription.UnreadCount += entriesWritten

  if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
    c.Errorf("Error writing subscription: %s", err)
    return err
  }

  return nil
}

func updateFeed(c appengine.Context, feedKey *datastore.Key, feed *storage.Feed) error {
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
      c.Warningf("UniqueID for an entry (title '%s') is missing", feed.Entries[i].Title)
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

func importSubscription(c appengine.Context, ch chan<- *opml.Subscription, userKey *datastore.Key, parentKey *datastore.Key, opmlSubscription *opml.Subscription) {
  url := opmlSubscription.URL
  feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
  feed := new(storage.Feed)

  var subscriptionKey *datastore.Key
  if err := datastore.Get(c, feedKey, feed); err != nil && err != datastore.ErrNoSuchEntity {
    // FIXME: handle error
    c.Errorf(err.Error())
    goto done
  } else if err == datastore.ErrNoSuchEntity {
    // Add the feed first
    client := urlfetch.Client(c)
    if resp, err := client.Get(url); err != nil {
      c.Errorf("Error fetching from URL %s: %s", url, err)
      // FIXME: handle error
      goto done
    } else {
      defer resp.Body.Close()
      if loadedFeed, err := rss.UnmarshalStream(url, resp.Body); err != nil {
        c.Errorf("Error parsing the feed stream for URL %s: %s", url, err)
        // FIXME: handle error
        goto done
      } else {
        feed, _ = storage.NewFeed(loadedFeed)
        if err := updateFeed(c, feedKey, feed); err != nil {
          c.Errorf("Error updating feed for URL %s: %s", url, err)
          // FIXME: handle error
          goto done
        }
      }
    }
  }

  if subKey, err := subscriptionKeyForURL(c, url, userKey); err != nil {
    c.Errorf("Error reading subscription: %s", err)
    // FIXME: handle error
    goto done
    return
  } else {
    if subKey != nil {
      c.Infof("Already subscribed to %s", url)
      goto done
    }
  }

  // Override the title with the one specified in the OPML file
  if opmlSubscription.Title != "" {
    feed.Title = opmlSubscription.Title
  }

  subscriptionKey = datastore.NewKey(c, "Subscription", url, 0, parentKey)
  if err := updateSubscription(c, subscriptionKey, feedKey, feed); err != nil {
    c.Errorf("Subscription update error: %s", err)
    // FIXME: handle error
    goto done
  }

done:
  ch<- opmlSubscription
}

func importSubscriptions(c appengine.Context, ch chan<- *opml.Subscription, userKey *datastore.Key, parentKey *datastore.Key, subscriptions []*opml.Subscription) int {
  count := 0
  for _, subscription := range subscriptions {
    if subscription.URL != "" {
      go importSubscription(c, ch, userKey, parentKey, subscription)
      count++
    }
    if subscription.Subscriptions != nil {
      // Find or create a folder
      var folderKey *datastore.Key

      q := datastore.NewQuery("Folder").Ancestor(userKey).Filter("Title =", subscription.Title).KeysOnly().Limit(1)
      if folderKeys, err := q.GetAll(c, nil); err == nil {
        if len(folderKeys) > 0 {
          // Found an existing folder with that name
          folderKey = folderKeys[0]
        } else {
          // Don't have a folder with that name - create a new one
          folder := storage.Folder {
            Title: subscription.Title,
          }

          folderKey = datastore.NewIncompleteKey(c, "Folder", userKey)
          if completeKey, err := datastore.Put(c, folderKey, &folder); err != nil {
            c.Errorf("Cannot write folder: %s", err)
            continue
          } else {
            folderKey = completeKey
          }
        }
      } else {
        // Some unplanned error - just skip the node
        c.Errorf("Cannot locate folder: %s", err)
        continue
      }

      count += importSubscriptions(c, ch, userKey, folderKey, subscription.Subscriptions)
    }
  }

  return count
}

func importOPMLTask(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  var userKey *datastore.Key
  if userID := r.FormValue("userID"); userID == "" {
    http.Error(w, "Missing userID", http.StatusInternalServerError)
    return
  } else {
    userKey = datastore.NewKey(c, "User", userID, 0, nil)
  }

  var doc opml.Document
  var blobKey appengine.BlobKey
  if blobKeyString := r.FormValue("opmlBlobKey"); blobKeyString == "" {
    http.Error(w, "Missing blob key", http.StatusInternalServerError)
    return
  } else {
    blobKey = appengine.BlobKey(blobKeyString)
  }

  reader := blobstore.NewReader(c, blobKey)
  if err := opml.Parse(reader, &doc); err != nil {
    http.Error(w, "Error reading OPML", http.StatusInternalServerError)

    // Remove the blob
    if err := blobstore.Delete(c, blobKey); err != nil {
      c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
    }
    return
  }
  
  // Remove the blob
  if err := blobstore.Delete(c, blobKey); err != nil {
    c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
  }

  importStarted := time.Now()

  doneChannel := make(chan *opml.Subscription)
  importing := importSubscriptions(c, doneChannel, userKey, userKey, doc.Subscriptions)

  for i := 0; i < importing; i++ {
    subscription := <-doneChannel;
    c.Infof("completed %s", subscription.Title)
  }

  c.Infof("completed all (took %s)", time.Since(importStarted))
}

func subscribeTask(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  var userKey *datastore.Key
  if userID := r.FormValue("userID"); userID == "" {
    http.Error(w, "Missing userID", http.StatusInternalServerError)
    return
  } else {
    userKey = datastore.NewKey(c, "User", userID, 0, nil)
  }

  url := r.FormValue("url")
  if url == "" {
    http.Error(w, "Missing URL", http.StatusInternalServerError)
    return
  }

  var folderKey *datastore.Key

  if folderId := r.PostFormValue("folderId"); folderId != "" {
    if kind, id, err := unformatId(folderId); err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    } else if kind == "folder" {
      folder := new(storage.Folder)
      folderKey = datastore.NewKey(c, "Folder", "", id, userKey)

      if err := datastore.Get(c, folderKey, folder); err == datastore.ErrNoSuchEntity {
        http.Error(w, "Folder not found", http.StatusInternalServerError)
        return
      } else if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
      }
    } else {
      http.Error(w, "Invalid ID kind", http.StatusInternalServerError)
      return
    }
  }

  feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
  feed := new(storage.Feed)

  if err := datastore.Get(c, feedKey, feed); err != nil && err != datastore.ErrNoSuchEntity {
    c.Errorf(err.Error())
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  } else if err == datastore.ErrNoSuchEntity {
    // Add the feed first
    client := urlfetch.Client(c)
    if resp, err := client.Get(url); err != nil {
      c.Errorf("Error fetching from URL %s: %s", url, err)
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    } else {
      defer resp.Body.Close()
      if loadedFeed, err := rss.UnmarshalStream(url, resp.Body); err != nil {
        c.Errorf("Error parsing the feed stream for URL %s: %s", url, err)
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
      } else {
        feed, _ = storage.NewFeed(loadedFeed)
        if err := updateFeed(c, feedKey, feed); err != nil {
          c.Errorf("Error updating feed for URL %s: %s", url, err)
          http.Error(w, err.Error(), http.StatusInternalServerError)
          return
        }
      }
    }
  }

  var ancestorKey *datastore.Key
  if folderKey != nil {
    ancestorKey = folderKey
  } else {
    ancestorKey = userKey
  }

  if subscriptionKey, err := subscriptionKeyForURL(c, url, userKey); err != nil {
    c.Errorf("Error reading subscription: %s", err)
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  } else {
    if subscriptionKey != nil {
      // Already subscribed; success
      return
    }
  }

  subscriptionKey := datastore.NewKey(c, "Subscription", url, 0, ancestorKey)
  if err := updateSubscription(c, subscriptionKey, feedKey, feed); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
}

func unsubscribeTask(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  // FIXME: broken

  var key *datastore.Key
  if encodedKey := r.PostFormValue("key"); encodedKey == "" {
    http.Error(w, "Missing key", http.StatusInternalServerError)
    return
  } else {
    if decodedKey, err := datastore.DecodeKey(encodedKey); err == nil {
      key = decodedKey
    } else {
      http.Error(w, "Error decoding key", http.StatusInternalServerError)
      return
    }
  }

  entriesRead := 0
  batchSize := 1000
  articleKeys := make([]*datastore.Key, batchSize)

  q := datastore.NewQuery("Article").Ancestor(key).KeysOnly()
  for t := q.Run(c); ; entriesRead++ {
    articleKey, err := t.Next(nil)

    if err == datastore.Done || entriesRead + 1 >= batchSize {
      // Delete the batch
      if entriesRead > 0 {
        if err := datastore.DeleteMulti(c, articleKeys[:entriesRead]); err != nil {
          http.Error(w, err.Error(), http.StatusInternalServerError)
          return
        }
      }

      entriesRead = 0

      if err == datastore.Done {
        break
      }
    } else if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }

    articleKeys[entriesRead] = articleKey
  }

  if key.Kind() == "Folder" {
    // Remove subscriptions under the folder
    q = datastore.NewQuery("Subscription").Ancestor(key).KeysOnly().Limit(1000)
    if subscriptionKeys, err := q.GetAll(c, nil); err == nil {
      if err := datastore.DeleteMulti(c, subscriptionKeys); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
      }
    }
  }

  if err := datastore.Delete(c, key); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
}

func markAllAsReadTask(w http.ResponseWriter, r *http.Request) {
  // c := appengine.NewContext(r)

  // var key *datastore.Key
  // if encodedKey := r.PostFormValue("key"); encodedKey == "" {
  //   http.Error(w, "Missing key", http.StatusInternalServerError)
  //   return
  // } else {
  //   if decodedKey, err := datastore.DecodeKey(encodedKey); err == nil {
  //     key = decodedKey
  //   } else {
  //     http.Error(w, "Error decoding key", http.StatusInternalServerError)
  //     return
  //   }
  // }

  // entriesRead := 0
  // batchSize := 1000
  // articleKeys := make([]*datastore.Key, batchSize)

  // FIXME
  // q := datastore.NewQuery("Article").Ancestor(key).KeysOnly()
  // for t := q.Run(c); ; entriesRead++ {
  //   articleKey, err := t.Next(nil)

  //   if err == datastore.Done || entriesRead + 1 >= batchSize {
  //     // Delete the batch
  //     if entriesRead > 0 {
  //       if err := datastore.DeleteMulti(c, articleKeys[:entriesRead]); err != nil {
  //         http.Error(w, err.Error(), http.StatusInternalServerError)
  //         return
  //       }
  //     }

  //     entriesRead = 0

  //     if err == datastore.Done {
  //       break
  //     }
  //   } else if err != nil {
  //     http.Error(w, err.Error(), http.StatusInternalServerError)
  //     return
  //   }

  //   articleKeys[entriesRead] = articleKey
  // }

  // if key.Kind() == "Folder" {
  //   // Remove subscriptions under the folder
  //   q = datastore.NewQuery("Subscription").Ancestor(key).KeysOnly().Limit(1000)
  //   if subscriptionKeys, err := q.GetAll(c, nil); err == nil {
  //     if err := datastore.DeleteMulti(c, subscriptionKeys); err != nil {
  //       http.Error(w, err.Error(), http.StatusInternalServerError)
  //       return
  //     }
  //   }
  // }

  // if err := datastore.Delete(c, key); err != nil {
  //   http.Error(w, err.Error(), http.StatusInternalServerError)
  //   return
  // }
}
