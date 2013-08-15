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
  "appengine/user"
  "appengine/taskqueue"
  "fmt"
  "net/url"
  "net/http"
  "time"
  "parser"

  "encoding/json"
)

type User struct {
  Key *datastore.Key `datastore:"-"`
  Joined time.Time
}

type Subscription struct {
  ID string `datastore:"-" json:"id"`
  Link string `datastore:"-" json:"link"`
  Title string `json:"title"`
  UnreadCount int `json:"unread"`
  Subscribed time.Time `json:"-"`
  Updated time.Time `json:"-"`
  Feed *datastore.Key `json:"-"`
}

type SubEntry struct {
  Retrieved time.Time
  Entry *datastore.Key
  Properties []string
}

type ReadableError struct {
  message string
  httpCode int
  err *error
}

var validProperties = map[string]bool {
  "read" : true,
  "star" : true,
  "like" : true,
}

func registerJson() {
  http.HandleFunc("/subscriptions", subscriptions)
  http.HandleFunc("/entries", entries)
  http.HandleFunc("/setProperty", setProperty)
  http.HandleFunc("/subscribe", subscribe)
}

func NewReadableError(message string, err *error) ReadableError {
  return ReadableError { message: message, httpCode: http.StatusInternalServerError, err: err }
}

func NewReadableErrorWithCode(message string, code int, err *error) ReadableError {
  return ReadableError { message: message, httpCode: code, err: err }
}

func (e ReadableError) Error() string {
  return e.message
}

func _l(format string, v ...interface {}) string {
  // FIXME
  return fmt.Sprintf(format, v...)
}

func writeError(c appengine.Context, w http.ResponseWriter, err error) {
  var message string
  var httpCode int

  if readableError, ok := err.(ReadableError); ok {
    message = err.Error() 
    httpCode = readableError.httpCode

    if readableError.err != nil {
      c.Errorf("Source error: %s", *readableError.err)
    }
  } else {
    message = _l("An unexpected error has occurred")
    httpCode = http.StatusInternalServerError

    c.Errorf("Error: %s", err)
  }

  jsonObj := map[string]string { "errorMessage": message }
  bf, _ := json.Marshal(jsonObj)

  w.Header().Set("Content-type", "application/json; charset=utf-8")
  http.Error(w, string(bf), httpCode)
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

  q := datastore.NewQuery("Subscription").Ancestor(userKey).Limit(1000)
  if subKeys, err := q.GetAll(c, &subscriptions); err != nil {
    writeError(c, w, err)
    return
  } else if subscriptions == nil {
    subscriptions = make([]*Subscription, 0)
  } else {
    subscriptionKeys = subKeys
  }

  feedKeys := make([]*datastore.Key, len(subscriptions))
  for i, subscription := range subscriptions {
    feedKeys[i] = subscription.Feed
  }

  feeds := make([]parser.Feed, len(subscriptions))
  if err := datastore.GetMulti(c, feedKeys, feeds); err != nil {
    writeError(c, w, err)
    return
  }

  for i, subscription := range subscriptions {
    feed := feeds[i]

    subscription.ID = subscriptionKeys[i].StringID()
    subscription.Link = feed.WWWURL
  }

  writeObject(w, subscriptions)
}

func getEntries(c appengine.Context, ancestorKey *datastore.Key) ([]parser.Entry, error) {
  var entries []parser.Entry

  q := datastore.NewQuery("SubEntry").Ancestor(ancestorKey).Order("-Retrieved").Limit(40)
  var subEntries []SubEntry

  if _, err := q.GetAll(c, &subEntries); err != nil {
    return nil, err
  } else {
    entries = make([]parser.Entry, len(subEntries))

    entryKeys := make([]*datastore.Key, len(subEntries))
    for i, subEntry := range subEntries {
      entryKeys[i] = subEntry.Entry
    }

    if err := datastore.GetMulti(c, entryKeys, entries); err != nil {
      return nil, err
    }

    for i, _ := range entries {
      entries[i].ID = entryKeys[i].StringID()
      entries[i].Source = entryKeys[i].Parent().StringID()
      entries[i].Properties = subEntries[i].Properties
    }
  }

  return entries, nil
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

  subscriptionID := r.FormValue("subscription")
  if subscriptionID == "" {
    writeError(c, w, NewReadableError(_l("Subscription not found"), nil))
    return
  }

  var ancestorKey *datastore.Key
  if subscriptionID == "" {
    ancestorKey = userKey
  } else {
    ancestorKey = datastore.NewKey(c, "Subscription", subscriptionID, 0, userKey)
  }

  if entries, err := getEntries(c, ancestorKey); err != nil {
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

  tagIndex := -1
  for i, property := range subEntry.Properties {
    if property == propertyName {
      tagIndex = i
      break
    }
  }

  writeChanges := true
  if setProp && tagIndex == -1 {
    subEntry.Properties = append(subEntry.Properties, propertyName)
  } else if !setProp && tagIndex != -1 {
    subEntry.Properties = append(subEntry.Properties[:tagIndex], subEntry.Properties[tagIndex + 1:]...)
  } else {
    writeChanges = false
  }

  if writeChanges {
    if _, err := datastore.Put(c, subEntryKey, subEntry); err != nil {
      writeError(c, w, NewReadableError(_l("Error updating article"), &err))
      return
    }

    if propertyName == "read" {
      // Update unread counts - not critical
      subscription := new(Subscription)
      if err := datastore.Get(c, subscriptionKey, subscription); err != nil {
        c.Errorf("Unread count update failed: subscription fetch error (%s)", err)
      } else {
        if !setProp {
          subscription.UnreadCount++
        } else {
          subscription.UnreadCount--
        }

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

  subscriptionUrl := r.PostFormValue("url")
  if subscriptionUrl == "" {
    writeError(c, w, NewReadableError(_l("URL unspecified"), nil))
    return
  }

  // FIXME: Verify if in system, or otherwise, if valid URL

  subscriptionKey := datastore.NewKey(c, "Subscription", subscriptionUrl, 0, userKey)
  subscription := new(Subscription)

  if err := datastore.Get(c, subscriptionKey, subscription); err == nil {
    // FIXME: provide title
    writeObject(w, map[string]string { "message": _l("You are already subscribed to %s", subscription.Title) })
    return
  } else if err != datastore.ErrNoSuchEntity {
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
