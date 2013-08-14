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
  "net/http"
  "time"
  "parser"
)

func subscribe(w http.ResponseWriter, r *http.Request) {
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
  feed := new(parser.Feed)

  if err := datastore.Get(c, feedKey, feed); err != nil && err != datastore.ErrNoSuchEntity {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  } else if err == datastore.ErrNoSuchEntity {
    // FIXME: add a new feed first
    http.Error(w, "FIXME: feed not in system", http.StatusInternalServerError)
    return
  }

  subscriptionKey := datastore.NewKey(c, "Subscription", url, 0, userKey)
  subscription := new(Subscription)

  if err := datastore.Get(c, subscriptionKey, subscription); err == nil {
    // Already subscribed; success
    return
  } else if err != datastore.ErrNoSuchEntity {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  // FIXME: write the entries

  var subscriptions []Subscription
  q := datastore.NewQuery("Subscription").Ancestor(&userKey).Limit(1000)
  if subscriptionKeys, err := q.GetAll(c, &subscriptions); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  } else {
    for i, subscription := range subscriptions {
      // FIXME: this would be a problem if the number of new records exceeded 1000
      q = datastore.NewQuery("Entry").Ancestor(subscription.Feed).Filter("Retrieved >", subscription.Updated).Order("Retrieved").KeysOnly().Limit(1000)
      if entryKeys, err := q.GetAll(c, nil); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
      } else {
        writeCount := len(entryKeys)
        subEntries := make([]SubEntry, writeCount)
        subEntryKeys := make([]*datastore.Key, writeCount)
        for j, entryKey := range entryKeys {
          subEntryKeys[j] = datastore.NewKey(c, "SubEntry", entryKey.StringID(), 0, subscriptionKeys[i])
          subEntries[j].Entry = entryKey
          subEntries[j].Created = time.Now().UTC()
        }
        if _, err := datastore.PutMulti(c, subEntryKeys, subEntries); err != nil {
          http.Error(w, err.Error(), http.StatusInternalServerError)
          return
        }

        if writeCount > 0 {
          lastEntry := new(parser.Entry)
          lastEntryKey := entryKeys[writeCount - 1]
          if err := datastore.Get(c, lastEntryKey, lastEntry); err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
          } else {
            subscription.Updated = lastEntry.Retrieved
            subscription.UnreadCount += writeCount
            
            if _, err := datastore.Put(c, subscriptionKeys[i], &subscription); err != nil {
              http.Error(w, err.Error(), http.StatusInternalServerError)
              return
            }
          }
        }
      }
    }
  }
  
  // Update the subscription

  subscription.Title = feed.Title
  subscription.Subscribed = time.Now().UTC()
  subscription.Updated = time.Time {}
  subscription.Feed = feedKey
  subscription.UnreadCount = 0

  if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
}

