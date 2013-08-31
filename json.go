/*****************************************************************************
 **
 ** PerFeediem
 ** https://github.com/melllvar/perfeediem
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
  "appengine/blobstore"
  "appengine/datastore"
  "appengine/taskqueue"
  "appengine/urlfetch"
  "appengine/user"
  "encoding/json"
  "fmt"
  "io/ioutil"
  "net/url"
  "net/http"
  "opml"
  "regexp"
  "rss"
  "storage"
  "strconv"
  "strings"
  "time"
  "unicode/utf8"
)

var validProperties = map[string]bool {
  "unread": true,
  "read":   true,
  "star":   true,
  "like":   true,
}

func registerJson() {
  RegisterRoute("/subscriptions", subscriptions)
  RegisterRoute("/articles",      articles)
  RegisterRoute("/createFolder",  createFolder)
  RegisterRoute("/rename",        rename)
  RegisterRoute("/setProperty",   setProperty)
  RegisterRoute("/subscribe",     subscribe)

  // FIXME: switch to custom router
  http.HandleFunc("/subscriptions", Run)
  http.HandleFunc("/articles",      Run)
  http.HandleFunc("/createFolder",  Run)
  http.HandleFunc("/rename",        Run)
  http.HandleFunc("/setProperty",   Run)
  http.HandleFunc("/subscribe",     Run)

  http.HandleFunc("/unsubscribe",   unsubscribe)
  http.HandleFunc("/import",        importOpml)
  http.HandleFunc("/authUpload",    authUpload)
  http.HandleFunc("/markAllAsRead", markAllAsRead)
}

func subscriptions(pfc *PFContext) (interface{}, error) {
  userID := storage.UserID(pfc.User.ID)

  return storage.NewUserSubscriptions(pfc.Context, userID)
}

func articles(pfc *PFContext) (interface{}, error) {
  r := pfc.R
  userID := storage.UserID(pfc.User.ID)

  filter := storage.ArticleFilter {
    SubscriptionID: r.FormValue("subscription"),
    FolderID: r.FormValue("folder"),
    UserID: userID,
  }

  if filterProperty := r.FormValue("filter"); validProperties[filterProperty] {
    filter.Property = filterProperty
  }

  return storage.NewArticlePage(pfc.Context, filter, r.FormValue("continue"))
}

func createFolder(pfc *PFContext) (interface{}, error) {
  r := pfc.R
  userID := storage.UserID(pfc.User.ID)

  title := r.PostFormValue("folderName")
  if title == "" {
    return nil, NewReadableError(_l("Missing folder name"), nil)
  }

  if utf8.RuneCountInString(title) > 200 {
    return nil, NewReadableError(_l("Folder name is too long"), nil)
  }

  if exists, err := storage.IsFolderTitleDuplicate(pfc.Context, userID, title); err != nil {
    return nil, err
  } else if exists {
    return nil, NewReadableError(_l("A folder with that name already exists"), nil)
  }

  if err := storage.CreateFolder(pfc.Context, userID, title); err != nil {
    return nil, NewReadableError(_l("An error occurred while adding the new folder"), &err)
  }

  return storage.NewUserSubscriptions(pfc.Context, userID)
}

func rename(pfc *PFContext) (interface{}, error) {
  r := pfc.R
  userID := storage.UserID(pfc.User.ID)

  title := r.PostFormValue("title")
  if title == "" {
    return nil, NewReadableError(_l("Name not specified"), nil)
  }

  folderID := r.PostFormValue("folder")

  if subscriptionID := r.PostFormValue("subscription"); subscriptionID != "" {
    // Rename subscription
    ref := storage.SubscriptionRef {
      FolderRef: storage.FolderRef {
        UserID: userID,
        FolderID: folderID,
      },
      SubscriptionID: subscriptionID,
    }
    if err := storage.RenameSubscription(pfc.Context, ref, title); err != nil {
      return nil, NewReadableError(_l("Error renaming folder"), &err)
    }
  } else if folderID != "" {
    // Rename folder
    if exists, err := storage.IsFolderTitleDuplicate(pfc.Context, userID, title); err != nil {
      return nil, err
    } else if exists {
      return nil, NewReadableError(_l("A folder with that name already exists"), nil)
    }

    if err := storage.RenameFolder(pfc.Context, userID, folderID, title); err != nil {
      return nil, NewReadableError(_l("Error renaming folder"), &err)
    }
  } else {
    return nil, NewReadableError(_l("Nothing to rename"), nil)
  }

  return storage.NewUserSubscriptions(pfc.Context, userID)
}

func setProperty(pfc *PFContext) (interface{}, error) {
  r := pfc.R
  userID := storage.UserID(pfc.User.ID)

  folderID := r.PostFormValue("folder")
  subscriptionID := r.PostFormValue("subscription")
  articleID := r.PostFormValue("article")
  propertyName := r.PostFormValue("property")
  propertyValue := r.PostFormValue("set") == "true"

  if articleID == "" || subscriptionID == "" {
    return nil, NewReadableError(_l("Article not found"), nil)
  }

  if !validProperties[propertyName] {
    return nil, NewReadableError(_l("Property not valid"), nil)
  }

  ref := storage.ArticleRef {
    SubscriptionRef: storage.SubscriptionRef {
      FolderRef: storage.FolderRef {
        UserID: userID,
        FolderID: folderID,
      },
      SubscriptionID: subscriptionID,
    },
    ArticleID: articleID,
  }

  if properties, err := storage.SetProperty(pfc.Context, ref, propertyName, propertyValue); err != nil {
    return nil, NewReadableError(_l("Error updating article"), &err)
  } else {
    return properties, nil
  }
}

func subscribe(pfc *PFContext) (interface{}, error) {
  c := pfc.C
  r := pfc.R
  userID := storage.UserID(pfc.User.ID)

  subscriptionURL := r.PostFormValue("url")
  folderId := r.PostFormValue("folder")

  if subscriptionURL == "" {
    return nil, NewReadableError(_l("Missing URL"), nil)
  } else if _, err := url.ParseRequestURI(subscriptionURL); err != nil {
    return nil, NewReadableError(_l("URL is not valid"), &err)
  }

  if folderId != "" {
    ref := storage.FolderRef {
      UserID: userID,
      FolderID: folderId,
    }

    if exists, err := storage.FolderExists(pfc.Context, ref); err != nil {
      return nil, err
    } else if !exists {
      return nil, NewReadableError(_l("Folder not found"), nil)
    }
  }

  if exists, err := storage.IsFeedAvailable(pfc.Context, subscriptionURL); err != nil {
    return nil, err
  } else if !exists {
    // Not a known feed URL
    // Match it against a list of known WWW links

    if feedURL, err := storage.WebToFeedURL(pfc.Context, subscriptionURL); err != nil {
      return nil, err
    } else if feedURL != "" {
      subscriptionURL = feedURL
    } else {
      // Still nothing
      // Add/remove 'www' to/from URL and try again

      var modifiedURL string
      if re := regexp.MustCompile(`://www\.`); re.MatchString(subscriptionURL) {
        modifiedURL = re.ReplaceAllString(subscriptionURL, "://")
      } else {
        re = regexp.MustCompile(`://`)
        modifiedURL = re.ReplaceAllString(subscriptionURL, "://www.")
      }

      if feedURL, err := storage.WebToFeedURL(pfc.Context, modifiedURL); err != nil {
        return nil, err
      } else if feedURL != "" {
        subscriptionURL = feedURL
      }
    }
  }

  if subscribed, err := storage.IsSubscribed(pfc.Context, userID, subscriptionURL); err != nil {
    return nil, err
  } else if subscribed {
    return nil, NewReadableError(_l("You are already subscribed"), nil)
  }

  // At this point, the URL may have been re-written, so we check again
  if exists, err := storage.IsFeedAvailable(pfc.Context, subscriptionURL); err != nil {
    return nil, err
  } else if !exists {
    // Don't have the locally - fetch it
    client := urlfetch.Client(c)
    if response, err := client.Get(subscriptionURL); err != nil {
      return nil, NewReadableError(_l("An error occurred while downloading the feed"), &err)
    } else {
      defer response.Body.Close()
      
      var body string
      if bytes, err := ioutil.ReadAll(response.Body); err != nil {
        return nil, NewReadableError(_l("An error occurred while reading the feed"), &err)
      } else {
        body = string(bytes)
      }

      reader := strings.NewReader(body)
      if _, err := rss.UnmarshalStream(subscriptionURL, reader); err != nil {
        // Parse failed. Assume it's an HTML document and 
        // try to pull out an RSS <link />

        if linkURL := rss.ExtractRSSLink(body); linkURL == "" {
          return nil, NewReadableError(_l("RSS content not found"), &err)
        } else {
          subscriptionURL = linkURL
        }
      }
    }
  }

  task := taskqueue.NewPOSTTask("/tasks/subscribe", url.Values {
    "url": { subscriptionURL },
    "folderId": { folderId },
    "userID": { pfc.User.ID },
  })

  if _, err := taskqueue.Add(c, task, ""); err != nil {
    return nil, NewReadableError(_l("Cannot subscribe - too busy"), &err)
  }

  return _l("Your subscription has been queued for addition."), nil
}

func _l(format string, v ...interface {}) string {
  // FIXME
  return fmt.Sprintf(format, v...)
}

func writeObject(w http.ResponseWriter, obj interface{}) {
  w.Header().Set("Content-type", "application/json; charset=utf-8")

  bf, _ := json.Marshal(obj)
  w.Write(bf)
}

func authorize(c appengine.Context, r *http.Request, w http.ResponseWriter) (*datastore.Key, error) {
  u := user.Current(c)
  if u == nil {
    err := NewReadableErrorWithCode(_l("Your session has expired. Please sign in."), http.StatusUnauthorized, nil)
    return nil, err
  }

  return datastore.NewKey(c, "User", u.ID, 0, nil), nil
}

func formatId(kind string, intId int64) string {
  return kind + "://" + strconv.FormatInt(intId, 36)
}

func unformatId(formattedId string) (string, int64, error) {
  if parts := strings.SplitN(formattedId, "://", 2); len(parts) == 2 {
    if id, err := strconv.ParseInt(parts[1], 36, 64); err == nil {
      return parts[0], id, nil
    } else {
      return parts[0], 0, nil
    }
  }

  return "", 0, NewReadableError(_l("Missing identifier"), nil)
}

func unsubscribe(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  var userKey *datastore.Key
  if u, err := authorize(c, r, w); err != nil {
    writeError(c, w, err)
    return
  } else {
    userKey = u
  }

  var task *taskqueue.Task
  if subscriptionURL := r.PostFormValue("subscription"); subscriptionURL != "" {
    // Remove a subscription
    ancestorKey := userKey
    if folder := r.PostFormValue("folder"); folder != "" {
      if kind, id, err := unformatId(folder); err != nil {
        writeError(c, w, err)
        return
      } else if kind == "folder" {
        ancestorKey = datastore.NewKey(c, "Folder", "", id, userKey)
      } else {
        writeError(c, w, NewReadableError(_l("Folder not found"), nil))
        return
      }
    }

    subscriptionKey := datastore.NewKey(c, "Subscription", subscriptionURL, 0, ancestorKey)
    subscription := new(storage.Subscription)

    if err := datastore.Get(c, subscriptionKey, subscription); err == datastore.ErrNoSuchEntity {
      writeError(c, w, NewReadableError(_l("Subscription not found"), nil))
      return
    } else if err != nil {
      writeError(c, w, err)
      return
    }

    task = taskqueue.NewPOSTTask("/tasks/unsubscribe", url.Values {
      "key": { subscriptionKey.Encode() },
    })
  } else if folder := r.PostFormValue("folder"); folder != "" {
    // Remove a folder
    var folderKey *datastore.Key
    if kind, id, err := unformatId(folder); err != nil {
      writeError(c, w, err)
      return
    } else if kind == "folder" {
      folderKey = datastore.NewKey(c, "Folder", "", id, userKey)
    } else {
      writeError(c, w, NewReadableError(_l("Folder not found"), nil))
      return
    }

    folder := new(storage.Folder)
    if err := datastore.Get(c, folderKey, folder); err == datastore.ErrNoSuchEntity {
      writeError(c, w, NewReadableError(_l("Folder not found"), nil))
      return
    } else if err != nil {
      writeError(c, w, err)
      return
    }

    task = taskqueue.NewPOSTTask("/tasks/unsubscribe", url.Values {
      "key": { folderKey.Encode() },
    })
  } else {
    writeError(c, w, NewReadableError(_l("Item not found"), nil))
    return
  }

  if _, err := taskqueue.Add(c, task, ""); err != nil {
    writeError(c, w, NewReadableError(_l("Error queueing deletion"), &err))
    return
  } else {
    writeObject(w, map[string]string { "message": _l("Queued for deletion") })
  }
}

func importOpml(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  blobs, _, err := blobstore.ParseUpload(r)
  if err != nil {
    writeError(c, w, NewReadableError(_l("Error receiving file"), &err))
    return
  }

  var blobKey appengine.BlobKey
  if blobInfos := blobs["opml"]; len(blobInfos) == 0 {
    writeError(c, w, NewReadableError(_l("File not uploaded"), nil))
    return
  } else {
    blobKey = blobInfos[0].BlobKey
    reader := blobstore.NewReader(c, blobKey)

    var doc opml.Document
    if err := opml.Parse(reader, &doc); err != nil {
      writeError(c, w, NewReadableError(_l("Error reading OPML file"), &err))

      // Remove the blob
      if err := blobstore.Delete(c, blobKey); err != nil {
        c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
      }
      return
    }
  }

  user := user.Current(c)
  task := taskqueue.NewPOSTTask("/tasks/import", url.Values {
    "opmlBlobKey": { string(blobKey) },
    "userID": { user.ID },
  })

  if _, err := taskqueue.Add(c, task, ""); err != nil {
    writeError(c, w, NewReadableError(_l("Error initiating import"), &err))
    
    // Remove the blob
    if err := blobstore.Delete(c, blobKey); err != nil {
      c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
    }
    return
  }

  writeObject(w, map[string]string { "message": _l("Your subscriptions have been queued for addition.") })
}

func authUpload(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  uploadURL, err := blobstore.UploadURL(c, "/import", nil)
  if err != nil {
    writeError(c, w, NewReadableError(_l("Error initiating file upload"), &err))
    return
  }

  writeObject(w, map[string]string { 
    "uploadUrl": uploadURL.String(),
  })
}

func markAllAsRead(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)

  var userKey *datastore.Key
  if u, err := authorize(c, r, w); err != nil {
    writeError(c, w, err)
    return
  } else {
    userKey = u
  }

  var key *datastore.Key
  if subscriptionURL := r.PostFormValue("subscription"); subscriptionURL != "" {
    // Subscription
    ancestorKey := userKey
    if folder := r.PostFormValue("folder"); folder != "" {
      if kind, id, err := unformatId(folder); err != nil {
        writeError(c, w, err)
        return
      } else if kind == "folder" {
        ancestorKey = datastore.NewKey(c, "Folder", "", id, userKey)
      } else {
        writeError(c, w, NewReadableError(_l("Folder not found"), nil))
        return
      }
    }

    key = datastore.NewKey(c, "Subscription", subscriptionURL, 0, ancestorKey)
  } else if folder := r.PostFormValue("folder"); folder != "" {
    // Folder
    var folderKey *datastore.Key
    if kind, id, err := unformatId(folder); err != nil {
      writeError(c, w, err)
      return
    } else if kind == "folder" {
      folderKey = datastore.NewKey(c, "Folder", "", id, userKey)
    } else {
      writeError(c, w, NewReadableError(_l("Folder not found"), nil))
      return
    }

    key = folderKey
  } else {
    key = userKey
  }

  q := datastore.NewQuery("Article").Ancestor(key).Filter("Properties =", "unread").Limit(1000)

  var articles []*storage.Article
  if articleKeys, err := q.GetAll(c, &articles); err != nil {
    writeError(c, w, err)
    return
  } else {
    if len(articleKeys) >= 1000 {
      // Too many entries - delegate job to a background task
      task := taskqueue.NewPOSTTask("/tasks/markAllAsRead", url.Values {
        "key": { key.Encode() },
      })
      if _, err := taskqueue.Add(c, task, ""); err != nil {
        writeError(c, w, err)
        return
      } else {
        writeObject(w, map[string]interface{} { 
          "message": _l("Queued for marking"),
          "done": false,
        })
      }
    } else {
      started := time.Now()

      for _, article := range articles {
        propertyMap := make(map[string]bool)
        for _, property := range article.Properties {
          propertyMap[property] = true
        }

        delete(propertyMap, "unread")
        propertyMap["read"] = true

        article.Properties = make([]string, len(propertyMap))
        i := 0
        for key, _ := range propertyMap {
          article.Properties[i] = key
          i++
        }
      }

      if _, err := datastore.PutMulti(c, articleKeys, articles); err != nil {
        writeError(c, w, NewReadableError(_l("Error marking items as read"), &err))
        // FIXME: deal with multi. errors
        return
      }

      itemsMarked := 0

      // Reset unread counter
      if key.Kind() != "Subscription" {
        var subscriptions []*storage.Subscription
        q = datastore.NewQuery("Subscription").Ancestor(key).Limit(1000)
        
        if subscriptionKeys, err := q.GetAll(c, &subscriptions); err == nil {
          for _, subscription := range subscriptions {
            itemsMarked += subscription.UnreadCount
            subscription.UnreadCount = 0
          }

          if _, err := datastore.PutMulti(c, subscriptionKeys, subscriptions); err != nil {
            writeError(c, w, NewReadableError(_l("Error writing unread item count"), &err))
            // FIXME: deal with multi. errors
            return
          }
        } else {
          writeError(c, w, NewReadableError(_l("Error reading unread item count"), &err))
          return
        }
      } else {
        subscription := new(storage.Subscription)
        if err := datastore.Get(c, key, subscription); err != nil {
          writeError(c, w, NewReadableError(_l("Error reading unread item count"), &err))
          return
        } else {
          itemsMarked += subscription.UnreadCount
          subscription.UnreadCount = 0

          if _, err := datastore.Put(c, key, subscription); err != nil {
            writeError(c, w, NewReadableError(_l("Error writing unread item count"), &err))
            return
          }
        }
      }

      c.Infof("Inline marked %d items as read (took %s)", itemsMarked, time.Since(started))

      writeObject(w, map[string]interface{} { 
        "message": _l(`%d items marked as read`, itemsMarked),
        "done": true,
      })
    }
  }
}
