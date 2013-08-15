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
    "html/template"
    "net/http"
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
