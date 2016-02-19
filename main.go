/*****************************************************************************
 **
 ** Gofr
 ** https://github.com/pokebyte/Gofr
 ** Copyright (C) 2013-2016 Akop Karapetyan
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

	pfc := PFContext {
		R: r,
		C: c,
		W: w,
		LoginURL: loginURL,
	}

	routeRequest(&pfc)
}
