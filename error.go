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
  "net/http"
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
