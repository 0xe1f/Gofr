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
 
package gofr

import (
  "appengine"
  "appengine/user"
  crand "crypto/rand"
  "github.com/gorilla/sessions"
  "io"
  mrand "math/rand"
  "net/http"
  "strconv"
)

var cookieStore *sessions.CookieStore

func init() {
  // Initialize cookie store
  bytes := make([]byte, 20)
  if n, err := io.ReadFull(crand.Reader, bytes); n != len(bytes) || err != nil {
  	// FIXME: Critical, failed to initialize random array of bytes
  	return
  }
  cookieStore = sessions.NewCookieStore(bytes)

  // Initialize handlers
  http.HandleFunc("/", Run)

  registerJson()
  registerTasks()
  registerCron()
  registerWeb()
}

type PFContext struct {
  R *http.Request
  C appengine.Context
  W http.ResponseWriter
  ClientID string
  User *user.User
  LoginURL string
}

func (context PFContext)ChannelID() string {
  return context.User.ID + "," + context.ClientID
}

func Run(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  loginURL := ""
  if url, err := user.LoginURL(c, r.URL.String()); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  } else {
    loginURL = url
  }

  var clientID string

  session, _ := cookieStore.Get(r, "main")
  if id, ok := session.Values["clientID"].(string); !ok || id == "" {
    clientID = strconv.Itoa(mrand.Int())
    session.Values["clientID"] = clientID
    session.Save(r, w)
  } else {
    clientID = id
  }

  pfc := PFContext {
    R: r,
    C: c,
    W: w,
    ClientID: clientID,
    User: user.Current(c),
    LoginURL: loginURL,
  }

  routeRequest(&pfc)
}
