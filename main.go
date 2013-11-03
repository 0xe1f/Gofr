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

const (
	requiredStorageVersion = 2
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

	currentStorageVersion int
	requiredStorageVersion int
}

func Run(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	storageVersion, err := storage.StorageVersion(c)
	if err != nil {
		c.Errorf("Storage version check error: %v", err)
		http.Error(w, "Error initializing storage", http.StatusInternalServerError)
		return
	} else if storageVersion < requiredStorageVersion {
		if r.URL.Path != "/tasks/migrate" {
			c.Errorf("Update storage first. As administrator, run /tasks/migrate (current: %d; required: %d)", 
				storageVersion, requiredStorageVersion)
			http.Error(w, "Storage must be migrated first. Check the error log", 
				http.StatusInternalServerError)
			
			return
		}
	}

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

		currentStorageVersion: storageVersion,
		requiredStorageVersion: requiredStorageVersion,
	}

	routeRequest(&pfc)
}
