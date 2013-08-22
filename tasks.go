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
  "time"
)

func registerTasks() {
  http.HandleFunc("/tasks/subscribe", subscribeTask)
  http.HandleFunc("/tasks/import", importOpmlTask)
}

func updateSubscription(c appengine.Context, subscriptionKey *datastore.Key, feedKey *datastore.Key, feed *Feed) error {
  batchSize := 1000
  mostRecentEntryTime := time.Time {}
  subEntries := make([]SubEntry, batchSize)
  subEntryKeys := make([]*datastore.Key, batchSize)
  entriesRead := 0
  entriesWritten := 0

  subscription := new(Subscription)
  if err := datastore.Get(c, subscriptionKey, subscription); err != nil && err != datastore.ErrNoSuchEntity {
    c.Errorf("Error getting subscription: %s", err)
    return err
  } else if err == datastore.ErrNoSuchEntity {
    // Set defaults
    subscription.Title = feed.Title
    subscription.Subscribed = time.Now().UTC()
    subscription.Feed = feedKey
  }

  err := datastore.RunInTransaction(c, func(c appengine.Context) error {
    q := datastore.NewQuery("EntryMeta").Ancestor(feedKey).Filter("Retrieved >", subscription.Updated)
    for t := q.Run(c); ; entriesRead++ {
      entryMeta := new(EntryMeta)
      _, err := t.Next(entryMeta)

      if err == datastore.Done || entriesRead + 1 >= batchSize {
        // Write the batch
        if entriesRead > 0 {
          if _, err := datastore.PutMulti(c, subEntryKeys[:entriesRead], subEntries[:entriesRead]); err != nil {
            c.Errorf("Error writing SubEntry batch: %s", err)
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

      subEntryKeys[entriesRead] = datastore.NewKey(c, "SubEntry", entryMeta.Entry.StringID(), 0, subscriptionKey)
      subEntries[entriesRead].Entry = entryMeta.Entry
      subEntries[entriesRead].Retrieved = entryMeta.Retrieved
      subEntries[entriesRead].Published = entryMeta.Published
      subEntries[entriesRead].Properties = []string { "unread" }

      mostRecentEntryTime = entryMeta.Retrieved
    }

    return nil
  }, &datastore.TransactionOptions { XG: true })

  if err != nil {
    c.Errorf("Entry transaction error (URL %s): %s", feed.URL, err)
    return err
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

func updateFeed(c appengine.Context, feedKey *datastore.Key, feed *Feed) error {
  batchSize := 1000
  elements := len(feed.Entries)

  entryKeys := make([]*datastore.Key, batchSize)
  entryMetaKeys := make([]*datastore.Key, batchSize)

  pending := 0
  written := 0

  err := datastore.RunInTransaction(c, func(c appengine.Context) error {
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
  }, nil)

  if err != nil {
    c.Errorf("Feed transaction error (URL %s): %s", feed.URL, err)
  }

  return err
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

  feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
  feed := new(Feed)

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
        feed, _ = NewFeed(loadedFeed)
        if err := updateFeed(c, feedKey, feed); err != nil {
          c.Errorf("Error updating feed for URL %s: %s", url, err)
          http.Error(w, err.Error(), http.StatusInternalServerError)
          return
        }
      }
    }
  }

  subscriptionKey := datastore.NewKey(c, "Subscription", url, 0, userKey)
  subscription := new(Subscription)

  if err := datastore.Get(c, subscriptionKey, &subscription); err == nil {
    // Already subscribed; success
    return
  } else if err != datastore.ErrNoSuchEntity {
    c.Errorf("Error reading subscription: %s", err)
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  if err := updateSubscription(c, subscriptionKey, feedKey, feed); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
}

func importSubscription(c appengine.Context, ch chan<- *opml.Subscription, userKey *datastore.Key, opmlSubscription *opml.Subscription) {
  url := opmlSubscription.URL
  feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
  feed := new(Feed)

  var subscriptionKey *datastore.Key
  var subscription Subscription

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
        feed, _ = NewFeed(loadedFeed)
        if err := updateFeed(c, feedKey, feed); err != nil {
          c.Errorf("Error updating feed for URL %s: %s", url, err)
          // FIXME: handle error
          goto done
        }
      }
    }
  }

  subscriptionKey = datastore.NewKey(c, "Subscription", url, 0, userKey)

  if err := datastore.Get(c, subscriptionKey, &subscription); err == nil {
    // Already subscribed; success
    c.Infof("Already subscribed to %s", url)
    goto done
  } else if err != datastore.ErrNoSuchEntity {
    c.Errorf("Error reading subscription: %s", err)
    // FIXME: handle error
    goto done
  }

  // Override the title with the one specified in the OPML file
  if opmlSubscription.Title != "" {
    feed.Title = opmlSubscription.Title
  }

  if err := updateSubscription(c, subscriptionKey, feedKey, feed); err != nil {
    c.Errorf("Subscription update error: %s", err)
    // FIXME: handle error
    goto done
  }

done:
  ch<- opmlSubscription
}

func importSubscriptions(c appengine.Context, ch chan<- *opml.Subscription, userKey *datastore.Key, subscriptions []*opml.Subscription) int {
  count := 0
  for _, subscription := range subscriptions {
    if subscription.URL != "" {
      go importSubscription(c, ch, userKey, subscription)
      count++
    }
    if subscription.Subscriptions != nil {
      count += importSubscriptions(c, ch, userKey, subscription.Subscriptions)
    }
  }

  return count
}

func importOpmlTask(w http.ResponseWriter, r *http.Request) {
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
  importing := importSubscriptions(c, doneChannel, userKey, doc.Subscriptions)

  for i := 0; i < importing; i++ {
    subscription := <-doneChannel;
    c.Infof("completed %s", subscription.Title)
  }

  c.Infof("completed all (took %s)", time.Since(importStarted))
}
