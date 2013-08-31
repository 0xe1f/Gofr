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
 "appengine/datastore"
 "appengine/user"
 "encoding/json"
 "net/http"
 "storage"
)

type PFContext struct {
  R *http.Request
  C appengine.Context
  Context storage.Context
  User *user.User
  UserKey *datastore.Key
}

func Run(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  user := user.Current(c)

  // FIXME: fancy pattern-match
  var matchingRoute *route
  if route := routeFromRequest(r); route == nil {
    // FIXME: routing error
    return
  } else if route.LoginRequired && user == nil {
    // FIXME: authorization error
    return
  } else {
    matchingRoute = route
  }

  var userKey *datastore.Key
  if user != nil {
    userKey = datastore.NewKey(c, "User", user.ID, 0, nil)
  }

  pfc := PFContext {
    R: r,
    C: c,
    Context: c,
    User: user,
    UserKey: userKey,
  }

  if returnValue, err := matchingRoute.Handler(&pfc); err == nil {
    var jsonObj interface{}
    if message, ok := returnValue.(string); ok {
      jsonObj = map[string]string { "message": message }
    } else {
      jsonObj = returnValue
    }

    bf, _ := json.Marshal(jsonObj)
    w.Header().Set("Content-type", "application/json; charset=utf-8")
    w.Write(bf)
  } else {
    message := _l("An unexpected error has occurred")
    httpCode := http.StatusInternalServerError

    c.Errorf("Error: %s", err)

    if readableError, ok := err.(ReadableError); ok {
      message = err.Error() 
      httpCode = readableError.httpCode

      if readableError.err != nil {
        c.Errorf("Source: %s", *readableError.err)
      }
    }

    jsonObj := map[string]string { "errorMessage": message }
    bf, _ := json.Marshal(jsonObj)

    w.Header().Set("Content-type", "application/json; charset=utf-8")
    http.Error(w, string(bf), httpCode)
  }
}
