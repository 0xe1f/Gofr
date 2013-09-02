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
 "encoding/json"
 "net/http"
)

type requestHandler interface {
  handleRequest(pfc *PFContext)
}

type route struct {
  Pattern string
  Handler requestHandler
}

type htmlRequestHandler struct {
  RouteHandler HTMLRouteHandler
  LoginRequired bool
}

type jsonRequestHandler struct {
  RouteHandler JSONRouteHandler
  LoginRequired bool
}

type taskRequestHandler struct {
  RouteHandler TaskRouteHandler
}

type HTMLRouteHandler func(pfc *PFContext)
type JSONRouteHandler func(pfc *PFContext) (interface{}, error)
type TaskRouteHandler func(pfc *PFContext) error

var routes []route = make([]route, 0, 100)

func (handler htmlRequestHandler)handleRequest(pfc *PFContext) {
  w := pfc.W

  if handler.LoginRequired && pfc.User == nil {
    w.Header().Set("Location", pfc.LoginURL)
    w.WriteHeader(http.StatusFound)
    return
  }

  handler.RouteHandler(pfc)
}

func (handler jsonRequestHandler)handleRequest(pfc *PFContext) {
  w := pfc.W
  c := pfc.C

  if handler.LoginRequired && pfc.User == nil {
    jsonObj := map[string]string { "errorMessage": _l("Please sign in") }
    bf, _ := json.Marshal(jsonObj)

    w.Header().Set("Content-type", "application/json; charset=utf-8")
    http.Error(w, string(bf), 401)
    return
  }

  if returnValue, err := handler.RouteHandler(pfc); err == nil {
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

func (handler taskRequestHandler)handleRequest(pfc *PFContext) {
  if err := handler.RouteHandler(pfc); err != nil {
    pfc.C.Errorf("Task failed: %s", err.Error())
    http.Error(pfc.W, err.Error(), http.StatusInternalServerError)
  }
}

func routeRequest(pfc *PFContext) {
  for _, route := range routes {
    if pfc.R.URL.Path == route.Pattern {
      route.Handler.handleRequest(pfc)
      return
    }
  }
}

func RegisterJSONRoute(pattern string, handler JSONRouteHandler) {
  route := route {
    Pattern: pattern,
    Handler: jsonRequestHandler {
      RouteHandler: handler,
      LoginRequired: true,
    },
  }

  routes = append(routes, route)
}

func RegisterHTMLRoute(pattern string, handler HTMLRouteHandler) {
  route := route {
    Pattern: pattern,
    Handler: htmlRequestHandler {
      RouteHandler: handler,
      LoginRequired: true,
    },
  }

  routes = append(routes, route)
}

func RegisterTaskRoute(pattern string, handler TaskRouteHandler) {
  route := route {
    Pattern: pattern,
    Handler: taskRequestHandler {
      RouteHandler: handler,
    },
  }

  routes = append(routes, route)
}
