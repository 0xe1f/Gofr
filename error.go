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
  "net/http"
  "encoding/json"
)

type ReadableError struct {
  message string
  httpCode int
  err *error
}

func NewReadableError(message string, err *error) ReadableError {
  return ReadableError { message: message, httpCode: http.StatusInternalServerError, err: err }
}

func NewReadableErrorWithCode(message string, code int, err *error) ReadableError {
  return ReadableError { message: message, httpCode: code, err: err }
}

func (e ReadableError) Error() string {
  return e.message
}

func writeError(c appengine.Context, w http.ResponseWriter, err error) {
  var message string
  var httpCode int

  if readableError, ok := err.(ReadableError); ok {
    message = err.Error() 
    httpCode = readableError.httpCode

    if readableError.err != nil {
      c.Errorf("Source error: %s", *readableError.err)
    }
  } else {
    message = _l("An unexpected error has occurred")
    httpCode = http.StatusInternalServerError

    c.Errorf("Error: %s", err)
  }

  jsonObj := map[string]string { "errorMessage": message }
  bf, _ := json.Marshal(jsonObj)

  w.Header().Set("Content-type", "application/json; charset=utf-8")
  http.Error(w, string(bf), httpCode)
}
