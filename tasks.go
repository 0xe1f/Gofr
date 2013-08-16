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
  "appengine/urlfetch"
  "net/http"
  "time"
  "parser"
)

func registerTasks() {
  http.HandleFunc("/tasks/subscribe", subscribeTask)
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

  q := datastore.NewQuery("EntryMeta").Ancestor(feedKey).Filter("Retrieved >", subscription.Updated)
  for t := q.Run(c); ; entriesRead++ {
    entryMeta := new(EntryMeta)
    _, err := t.Next(entryMeta)

    if err == datastore.Done || entriesRead + 1 >= batchSize {
      c.Infof("Writing batch; %d elements out of %d", entriesRead, batchSize)
      
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

func updateFeed(c appengine.Context, feedKey *datastore.Key, feed *Feed) error {
  batchSize := 1000
  elements := len(feed.Entries)

  entryKeys := make([]*datastore.Key, batchSize)
  entryMetaKeys := make([]*datastore.Key, batchSize)

  pending := 0
  written := 0

  // FIXME: transactions

  for i := 0; ; i++ {
    if i >= elements || pending + 1 >= batchSize {
      c.Infof("Writing batch; %d elements out of %d", pending, batchSize)

      if pending > 0 {
        if _, err := datastore.PutMulti(c, entryKeys[:pending], feed.Entries[written:written + pending]); err != nil {
          c.Errorf("Error writing Entry batch: %s", err)
          return err
        }
        if _, err := datastore.PutMulti(c, entryMetaKeys[:pending], feed.EntryMetas[written:written + pending]); err != nil {
          c.Errorf("Error writing Entry batch: %s", err)
          return err
        }
      }

      written += pending
      pending = 0

      if i >= elements {
        break
      }
    }

    keyName := feed.Entries[i].GUID // FIXME: figure out a better guid scheme

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
    // FIXME: this may be problematic if two people add same nonexistent subscription
    // at once

    /*
    err := datastore.RunInTransaction(c, func(c appengine.Context) error {
        var err1 error
        count, err1 = inc(c, datastore.NewKey(c, "Counter", "singleton", 0, nil))
        return err1
    }, nil)
    */
    client := urlfetch.Client(c)
    if resp, err := client.Get(url); err != nil {
      c.Errorf("Error fetching from URL %s: %s", url, err)
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    } else {
      defer resp.Body.Close()
      if loadedFeed, err := parser.UnmarshalStream(url, resp.Body); err != nil {
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
  if err := datastore.Get(c, subscriptionKey, nil); err == nil {
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
