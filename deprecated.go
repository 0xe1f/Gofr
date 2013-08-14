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
    "html/template"
    "net/http"
    "time"
    "parser"

    "fmt"
    "encoding/json"
    "appengine/urlfetch"
)

func parse(c appengine.Context, URL string) (*parser.Feed, error) {
  client := urlfetch.Client(c)
  resp, err := client.Get(URL)
  
  if err != nil {
    return nil, err
  }

  defer resp.Body.Close()

  return parser.UnmarshalStream(URL, resp.Body)
}

func root(w http.ResponseWriter, r *http.Request) {
    if err := rootTemplate.Execute(w, nil); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func addFeed(w http.ResponseWriter, r *http.Request) {
    if err := addFeedTemplate.Execute(w, nil); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

var rootTemplate = template.Must(template.New("book").Parse(rootTemplateHTML))
var addFeedTemplate = template.Must(template.New("book").Parse(addFeedTemplateHTML))
var addSubTemplate = template.Must(template.New("book").Parse(addSubTemplateHTML))

const rootTemplateHTML = `
<html>
  <body>
    <a href="/subscriptions">List Subs</a>
    <a href="/entries">List Entries</a>
<br/>
<br/>
    <a href="/addFeed">Add Feed</a>
    <a href="/addSub">Add Sub</a>
    <a href="/refresh">Refresh</a>
  </body>
</html>
`

const addFeedTemplateHTML = `
<html>
  <body>
    <form action="/doAddFeed" method="post">
      <div><input name="url" type="text" style="width: 50em;" /></div>
      <div><input type="submit" value="Add Feed"></div>
    </form>
  </body>
</html>
`

const addSubTemplateHTML = `
<html>
  <body>
    <form action="/doAddSub" method="post">
      <div><input name="url" type="text" style="width: 50em;" /></div>
      <div><input type="submit" value="Add Subscription"></div>
    </form>
  </body>
</html>
`

func ensureUser(c appengine.Context, userKey *datastore.Key, u *user.User) (*User, error) {
  k := datastore.NewKey(c, "User", u.ID, 0, nil)
  e := new(User)

  if err := datastore.Get(c, k, e); err != nil {
    e.Joined = time.Now().UTC()
    if _, err := datastore.Put(c, k, e); err != nil {
      return nil, err
    }
  }

  *userKey = *k
  return e, nil
}

func refresh(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  u := user.Current(c)
  if u == nil {
    url, err := user.LoginURL(c, r.URL.String())
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }
    w.Header().Set("Location", url)
    w.WriteHeader(http.StatusFound)
    return
  }

  var userKey datastore.Key
  if _, err := ensureUser(c, &userKey, u); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

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
  fmt.Fprintf(w, "done")
}

func addSub(w http.ResponseWriter, r *http.Request) {
    c := appengine.NewContext(r)
    u := user.Current(c)
    if u == nil {
        url, err := user.LoginURL(c, r.URL.String())
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        w.Header().Set("Location", url)
        w.WriteHeader(http.StatusFound)
        return
    }

    var userKey datastore.Key
    if _, err := ensureUser(c, &userKey, u); err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }
    
    if err := addSubTemplate.Execute(w, nil); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func doAddSub(w http.ResponseWriter, r *http.Request) {
    c := appengine.NewContext(r)
    u := user.Current(c)
    if u == nil {
        url, err := user.LoginURL(c, r.URL.String())
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        w.Header().Set("Location", url)
        w.WriteHeader(http.StatusFound)
        return
    }

    url := r.FormValue("url")

    var userKey datastore.Key
    if _, err := ensureUser(c, &userKey, u); err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }

    fk := datastore.NewKey(c, "Feed", url, 0, nil)
    fe := new(parser.Feed)

    if err := datastore.Get(c, fk, fe); err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }

    q := datastore.NewQuery("Subscription").Ancestor(&userKey).Filter("Feed =", fk)
    if count, err := q.Count(c); err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
    } else {
      if count > 0 {
        http.Error(w, "Already subscribed", http.StatusInternalServerError)
      } else {
        se := new (Subscription)
        se.Title = fe.Title
        se.Subscribed = time.Now().UTC()
        se.Updated = time.Time {}
        se.Feed = fk

        sk := datastore.NewKey(c, "Subscription", url, 0, &userKey)
        if _, err := datastore.Put(c, sk, se); err != nil {
          http.Error(w, err.Error(), http.StatusInternalServerError)
        }
      }
    }

    w.Header().Set("Location", "/listSubs")
    w.WriteHeader(http.StatusFound)
}

func doAddFeed(w http.ResponseWriter, r *http.Request) {
    c := appengine.NewContext(r)

    url := r.FormValue("url")

    k := datastore.NewKey(c, "Feed", url, 0, nil)
    e := new(parser.Feed)

    if err := datastore.Get(c, k, e); err != nil {
      if feed, err := parse(c, url); err == nil {
        e = feed
        if _, err := datastore.Put(c, k, e); err != nil {
          http.Error(w, err.Error(), http.StatusInternalServerError)
        }
        ek := make([]*datastore.Key, len(feed.Entry))
        for i, entry := range feed.Entry {
          ek[i] = datastore.NewKey(c, "Entry", entry.GUID, 0, k)
        }
        if _, err := datastore.PutMulti(c, ek, feed.Entry); err != nil {
          http.Error(w, err.Error(), http.StatusInternalServerError)
        }
      } else {
        http.Error(w, err.Error(), http.StatusInternalServerError)
      }
    }

    bf, _ := json.MarshalIndent(e, "", "  ")
    fmt.Fprintf(w, string(bf))
}
