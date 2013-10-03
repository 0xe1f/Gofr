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
  "appengine/user"
  "encoding/xml"
  "fmt"
  "html/template"
  "io/ioutil"
  "net/http"
  "net/url"
  "storage"
  "time"
)

func registerWeb() {
  RegisterHTMLRoute("/reader", reader)
  RegisterHTMLRoute("/favicon", favicon)
  RegisterHTMLRoute("/export",  exportOPML)

  RegisterAnonHTMLRoute("/",    intro)
}

var readerTemplate = template.Must(template.New("reader").Parse(readerTemplateHTML))
var introTemplate = template.Must(template.New("intro").Parse(introTemplateHTML))

func intro(pfc *PFContext) {
  if pfc.User != nil {
    pfc.W.Header().Set("Location", "/reader")
    pfc.W.WriteHeader(http.StatusFound)
    return
  }

  if err := introTemplate.Execute(pfc.W, nil); err != nil {
    http.Error(pfc.W, err.Error(), http.StatusInternalServerError)
  }
}

func reader(pfc *PFContext) {
  content := map[string]string {
    "UserEmail": pfc.User.EmailAddress,
  }
  if logoutURL, err := user.LogoutURL(pfc.C, "/"); err == nil {
    content["LogOutURL"] = logoutURL
  }

  if err := readerTemplate.Execute(pfc.W, content); err != nil {
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

  if favIconURL == "" {
    http.Error(w, "Missing URL", http.StatusInternalServerError)
    return
  }

  if _, err := url.ParseRequestURI(favIconURL); err != nil {
    http.Error(w, "Invalid URL", http.StatusInternalServerError)
    return
  }

  if favIcon, err := findFavIcon(c, favIconURL); err == nil {
    if favIcon.LastUpdated.IsZero() {
      // Save the new icon
      favIcon.LastUpdated = time.Now()
      if err := favIcon.Save(c, favIconURL); err != nil {
        c.Warningf("Error saving favicon '%s': %s", favIconURL, err)
      }
    }

    w.Header().Set("Cache-control", "public, max-age=2628000")
    w.Header().Set("Content-type", favIcon.MimeType)
    w.Write(favIcon.Data)

    return
  }

  http.Error(w, "No image found", http.StatusInternalServerError)
}

func defaultFavIcon(c appengine.Context) (*storage.FavIcon, error) {
  favIconURL := "gofr://favicon/default"
  var defaultFavIcon *storage.FavIcon

  // Attempt a load from the data store
  if favIcon, err := storage.LoadFavIcon(c, favIconURL); err == nil && favIcon != nil {
    defaultFavIcon = favIcon
  } else if err != nil {
    c.Errorf("Error loading favicon from store: %s", err)
    return nil, err
  } else {
    // Not in the store. Load it from the FS and save it to the store
    if bytes, err := ioutil.ReadFile("res/favicon-default.png"); err != nil {
      c.Errorf("Error loading favicon from file system: %s", err)
      return nil, err
    } else {
      defaultFavIcon = &storage.FavIcon {
        Data: bytes,
        LastUpdated: time.Now(),
        MimeType: "image/png",
      }
      
      if err := defaultFavIcon.Save(c, favIconURL); err != nil {
        c.Warningf("Error saving default favicon: %s", err)
      }
    }
  }

  defaultFavIcon.LastUpdated = time.Time{}
  return defaultFavIcon, nil
}

func findFavIcon(c appengine.Context, favIconURL string) (*storage.FavIcon, error) {
  // Attempt loading from storage
  if favIcon, err := storage.LoadFavIcon(c, favIconURL); err == nil && favIcon != nil {
    return favIcon, nil
  } else if err != nil {
    return nil, err
  }

  // No local favicon - fetch from WWW
  if favIcon, err := fetchFavIcon(c, favIconURL); err == nil && favIcon != nil {
    return favIcon, nil
  }

  return defaultFavIcon(c)
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

func exportOPML(pfc *PFContext) {
  c := pfc.C
  w := pfc.W

  if opml, err := storage.SubscriptionsAsOPML(c, pfc.UserID); err != nil {
    c.Errorf("Error retrieving list of subscriptions: %s", err)
    http.Error(w, _l("Error retrieving list of subscriptions"), http.StatusInternalServerError)
    return
  } else {
    opml.SetTitle(_l("Gofr subscriptions for %s", pfc.User.EmailAddress))

    if output, err := xml.MarshalIndent(opml, "", "    "); err != nil {
      c.Errorf("Error generating XML: %s", err)
      http.Error(w, _l("Error generating subscriptions"), http.StatusInternalServerError)
    } else {
      w.Header().Set("Content-disposition", "attachment; filename=subscriptions.xml");
      w.Header().Set("Content-type", "application/xml; charset=utf-8")

      w.Write([]byte(xml.Header))
      w.Write(output)
    }
  }
}
