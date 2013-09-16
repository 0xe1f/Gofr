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
  "net/http"
  "storage"
)

func init() {
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
  ChannelID string
  UserID storage.UserID
  User *storage.User
  LoginURL string
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

  var gofrUser *storage.User

  if aeUser := user.Current(c); aeUser != nil {
    if u, err := storage.UserByID(c, aeUser.ID); err != nil {
      c.Errorf("Error loading user")
      http.Error(w, "Unexpected error - try again later", http.StatusInternalServerError)
      return
    } else if u == nil {
      // New user
      newUser := storage.User {
        ID: aeUser.ID,
      }
      if err := newUser.Save(c); err != nil {
        c.Errorf("Error saving new user")
        http.Error(w, "Unexpected error - try again later", http.StatusInternalServerError)
        return
      }
      gofrUser = &newUser
    } else {
      gofrUser = u
    }
  }

  pfc := PFContext {
    R: r,
    C: c,
    W: w,
    User: gofrUser,
    LoginURL: loginURL,
  }

  routeRequest(&pfc)
}
