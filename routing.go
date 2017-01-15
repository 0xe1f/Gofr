/*****************************************************************************
 **
 ** Gofr
 ** https://github.com/pokebyte/Gofr
 ** Copyright (C) 2013-2017 Akop Karapetyan
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
	"appengine/channel"
	"appengine/user"
	"encoding/json"
	"net/http"
	"storage"
)

type requestHandler interface {
	handleRequest(pfc *PFContext)
}

type route struct {
	Pattern string
	Handler requestHandler
}

type HTMLRouteHandler func(pfc *PFContext)
type CronRouteHandler func(pfc *PFContext) error
type JSONRouteHandler func(pfc *PFContext) (interface{}, error)
type TaskRouteHandler func(pfc *PFContext) (TaskMessage, error)

type htmlRequestHandler struct {
	RouteHandler HTMLRouteHandler
	LoginRequired bool
}

type jsonRequestHandler struct {
	RouteHandler JSONRouteHandler
	LoginRequired bool
	NoFormPreparse bool
}

type taskRequestHandler struct {
	RouteHandler TaskRouteHandler
}

type cronRequestHandler struct {
	RouteHandler CronRouteHandler
}

type TaskMessage struct {
	Message string   `json:"message"`
	Refresh bool     `json:"refresh"`
	Silent bool      `json:"-"`
	Code int         `json:"code,omitempty"`
	Subscriptions interface{} `json:"subscriptions,omitempty"`
}

var routes []route = make([]route, 0, 100)

func (handler htmlRequestHandler)handleRequest(pfc *PFContext) {
	w := pfc.W

	aeUser := user.Current(pfc.C)
	if handler.LoginRequired && aeUser == nil {
		w.Header().Set("Location", pfc.LoginURL)
		w.WriteHeader(http.StatusFound)
		return
	} else if aeUser != nil {
		pfc.UserID = storage.UserID(aeUser.ID)
		if user, err := loadUser(pfc.C, aeUser); err != nil {
			pfc.C.Errorf("Error loading user: %s", err)
			http.Error(w, "Unexpected error", http.StatusInternalServerError)
			return
		} else {
			pfc.User = user
		}
	}

	handler.RouteHandler(pfc)
}

func (handler jsonRequestHandler)handleRequest(pfc *PFContext) {
	w := pfc.W
	c := pfc.C

	aeUser := user.Current(pfc.C)
	if handler.LoginRequired && aeUser == nil {
		jsonObj := map[string]string { "errorMessage": _l("Please sign in") }
		bf, _ := json.Marshal(jsonObj)

		w.Header().Set("Content-type", "application/json; charset=utf-8")
		http.Error(w, string(bf), 401)
		return
	} else if aeUser != nil {
		pfc.UserID = storage.UserID(aeUser.ID)
		if !handler.NoFormPreparse {
			if clientID := pfc.R.PostFormValue("client"); clientID != "" {
				pfc.ChannelID = aeUser.ID + "," + clientID
			}
		}

		if user, err := loadUser(pfc.C, aeUser); err != nil {
			c.Errorf("Error loading user: %s", err)
			http.Error(w, "Unexpected error", http.StatusInternalServerError)
			return
		} else {
			pfc.User = user
		}
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
	if userID := pfc.R.PostFormValue("userID"); userID != "" {
		pfc.UserID = storage.UserID(userID)
		if channelID := pfc.R.PostFormValue("channelID"); channelID != "" {
			pfc.ChannelID = channelID
		}

		if user, err := storage.UserByID(pfc.C, pfc.UserID); err != nil {
			pfc.C.Errorf("Error loading user: %s", err)
			http.Error(pfc.W, "Unexpected error", http.StatusInternalServerError)
			return
		} else {
			pfc.User = user
		}
	}

	var response interface{}
	taskMessage, err := handler.RouteHandler(pfc)
	if err != nil {
		pfc.C.Errorf("Task failed: %s", err.Error())
		http.Error(pfc.W, err.Error(), http.StatusInternalServerError)
		response = map[string] string { "error": err.Error() }
	} else {
		response = taskMessage
	}

	if !taskMessage.Silent {
		if channelID := pfc.R.PostFormValue("channelID"); channelID != "" {
			if err := channel.SendJSON(pfc.C, channelID, response); err != nil {
				pfc.C.Criticalf("Error writing to channel: %s", err)
			}
		} else {
			pfc.C.Warningf("Channel ID is empty!")
		}
	}
}

func (handler cronRequestHandler)handleRequest(pfc *PFContext) {
	err := handler.RouteHandler(pfc)
	if err != nil {
		pfc.C.Errorf("Cron failed: %s", err.Error())
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

	if appengine.IsDevAppServer() {
		pfc.C.Warningf("Error routing %s: no destination", pfc.R.URL.Path)
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

func RegisterJSONRouteSansPreparse(pattern string, handler JSONRouteHandler) {
	route := route {
		Pattern: pattern,
		Handler: jsonRequestHandler {
			RouteHandler: handler,
			LoginRequired: true,
			NoFormPreparse: true,
		},
	}

	routes = append(routes, route)
}

func RegisterAnonHTMLRoute(pattern string, handler HTMLRouteHandler) {
	route := route {
		Pattern: pattern,
		Handler: htmlRequestHandler {
			RouteHandler: handler,
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

func RegisterCronRoute(pattern string, handler CronRouteHandler) {
	route := route {
		Pattern: pattern,
		Handler: cronRequestHandler {
			RouteHandler: handler,
		},
	}

	routes = append(routes, route)
}

func loadUser(c appengine.Context, user *user.User) (*storage.User, error) {
	if u, err := storage.UserByID(c, storage.UserID(user.ID)); err != nil {
		return nil, err
	} else if u == nil {
		// New user
		newUser := storage.User {
			ID: user.ID,
			EmailAddress: user.Email,
		}
		if err := newUser.Save(c); err != nil {
			return nil, err
		}

		return &newUser, nil
	} else {
		return u, nil
	}
}
