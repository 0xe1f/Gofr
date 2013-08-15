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
  "net/http"
)

func init() {
  registerJson()
  registerTasks()
  registerWeb()

  // Deprecated
  http.HandleFunc("/root", root)
  http.HandleFunc("/addFeed", addFeed)
  http.HandleFunc("/doAddFeed", doAddFeed)
}

func registerWeb() {
  http.HandleFunc("/", index)
}

func index(w http.ResponseWriter, r *http.Request) {
  if err := indexTemplate.Execute(w, nil); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
  }
}
