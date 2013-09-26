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
  "fmt"
  "html/template"
  "io/ioutil"
  "net/http"
  "net/url"
  "storage"
  "time"
)

func registerWeb() {
  RegisterHTMLRoute("/", index)
  RegisterHTMLRoute("/favicon", favicon)
}

var indexTemplate = template.Must(template.New("index").Parse(indexTemplateHTML))

func index(pfc *PFContext) {
  if err := indexTemplate.Execute(pfc.W, nil); err != nil {
    http.Error(pfc.W, err.Error(), http.StatusInternalServerError)
  }
}

func favicon(pfc *PFContext) {
  c := pfc.C
  r := pfc.R
  w := pfc.W

  var favIconURL string
  if requestedURL := r.FormValue("url"); requestedURL != "" {
    if url, err := url.Parse(requestedURL); err == nil {
      favIconURL = fmt.Sprintf("%s://%s/favicon.ico", url.Scheme, url.Host)
    }
  }

  if favIconURL != "" {
    if _, err := url.ParseRequestURI(favIconURL); err == nil {
      if favIcon, err := findFavIcon(c, favIconURL); err != nil {
        c.Errorf("Error loading image %s: %s", favIconURL, err)
      } else if favIcon != nil {
        if favIcon.LastUpdated.IsZero() {
          // Save the new icon
          if err := favIcon.Save(c, favIconURL); err != nil {
            c.Warningf("Error saving favicon '%s': %s", favIconURL, err)
          }
        }

        w.Header().Set("Content-type", favIcon.MimeType)
        w.Write(favIcon.Data)
        return
      }
    }
  }

  // FIXME: serve; don't redirect
  w.Header().Set("Location", "/content/default-favicon.png")
  w.WriteHeader(http.StatusFound)
}

func fetchFavIcon(c appengine.Context, favIconURL string) (*storage.FavIcon, error) {
  if response, err := urlfetch.Client(c).Get(favIconURL); err == nil {
    defer response.Body.Close()

    bytes, err := ioutil.ReadAll(response.Body)
    if err != nil {
      return nil, err
    }

    mimeType := http.DetectContentType(bytes)
    acceptedMimeTypes := []string {
      "image/vnd.microsoft.icon",
      "image/png",
      "image/gif",
    }

    for _, acceptedMimeType := range acceptedMimeTypes {
      if mimeType == acceptedMimeType {
        return &storage.FavIcon {
          Data: bytes,
          MimeType: mimeType,
          LastUpdated: time.Time{}, // Needs to be saved
        }, nil
      }
    }
  } else {
    return nil, err
  }

  // No image available
  return nil, nil
}

func findFavIcon(c appengine.Context, favIconURL string) (*storage.FavIcon, error) {
  // Attempt loading from storage
  if favIcon, err := storage.LoadFavIcon(c, favIconURL); err == nil && favIcon != nil {
    return favIcon, nil
  } else if err != nil {
    return nil, err
  }

  // No local favicon - fetch from WWW
  return fetchFavIcon(c, favIconURL)
}
