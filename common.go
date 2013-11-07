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
	"appengine/urlfetch"
	"net/http"
	"time"
)

const (
	fetchDeadlineSeconds = 12
)

var validProperties = map[string]bool {
	"unread": true,
	"read":   true,
	"star":   true,
	"like":   true,
}

func createHttpClient(context appengine.Context) *http.Client {
	return &http.Client {
		Transport: &urlfetch.Transport {
			Context: context,
			Deadline: time.Duration(fetchDeadlineSeconds) * time.Second,
		},
	}
}
