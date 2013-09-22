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
  "appengine/blobstore"
  "appengine/channel"
  "appengine/urlfetch"
  "io/ioutil"
  "net/url"
  "opml"
  "regexp"
  "rss"
  "storage"
  "strings"
  "time"
  "unicode/utf8"
)

const (
  subscriptionQueue = "subscriptions"
  importQueue = "imports"
  refreshQueue = "refreshes"
  modificationQueue = "modifications"
)

const (
  subscriptionStalePeriodInMinutes = 5
)

var validProperties = map[string]bool {
  "unread": true,
  "read":   true,
  "star":   true,
  "like":   true,
}

func registerJson() {
  RegisterJSONRoute("/subscriptions", subscriptions)
  RegisterJSONRoute("/articles",      articles)
  RegisterJSONRoute("/createFolder",  createFolder)
  RegisterJSONRoute("/rename",        rename)
  RegisterJSONRoute("/setProperty",   setProperty)
  RegisterJSONRoute("/subscribe",     subscribe)
  RegisterJSONRoute("/unsubscribe",   unsubscribe)
  RegisterJSONRoute("/markAllAsRead", markAllAsRead)
  RegisterJSONRoute("/moveSubscription", moveSubscription)
  RegisterJSONRoute("/removeFolder",  removeFolder);

  RegisterJSONRoute("/authUpload",    authUpload)
  RegisterJSONRoute("/initChannel",   initChannel)

  // PostFormValue before blobstore.ParseUpload results in
  // "blobstore: error reading next mime part with boundary",
  // so we read post form values after parsing the uploaded file
  RegisterJSONRouteSansPreparse("/import",        importOPML)
}

func subscriptions(pfc *PFContext) (interface{}, error) {
  c := pfc.C

  userSubscriptions, err := storage.NewUserSubscriptions(c, pfc.UserID)
  if err != nil {
    return nil, err
  }

  staleDuration := time.Duration(subscriptionStalePeriodInMinutes) * time.Minute
  if appengine.IsDevAppServer() {
    // On dev server, stale period is 1 minute
    staleDuration = time.Duration(1) * time.Minute
  }

  if time.Since(pfc.User.LastSubscriptionUpdate) > staleDuration {
    pfc.User.LastSubscriptionUpdate = time.Now()
    if err := pfc.User.Save(c); err != nil {
      c.Warningf("Could not write user object back to store: %s", err)
    } else {
      started := time.Now()

      // Determine if new feeds are available
      if needRefresh, err := storage.AreNewEntriesAvailable(c, userSubscriptions.Subscriptions); err != nil {
        c.Warningf("Could not determine if new entries are available: %s", err)
      } else if needRefresh {
        if appengine.IsDevAppServer() {
          c.Debugf("Subscriptions need update; initiating a refresh (took %s)", time.Since(started))
        }

        if err := startTask(pfc, "refresh", nil, refreshQueue); err != nil {
          c.Warningf("Could not initiate the refresh task: %s", err)
        }
      } else {
        if appengine.IsDevAppServer() {
          c.Debugf("Subscriptions are up to date (took %s)", time.Since(started))
        }
      }
    }
  }

  return userSubscriptions, nil
}

func articles(pfc *PFContext) (interface{}, error) {
  r := pfc.R

  filter := storage.ArticleFilter {
    ArticleScope: storage.ArticleScope {
      FolderRef: storage.FolderRef {
        UserID: pfc.UserID,
        FolderID: r.FormValue("folder"),
      },
      SubscriptionID: r.FormValue("subscription"),
    },
  }

  if filterProperty := r.FormValue("filter"); validProperties[filterProperty] {
    filter.Property = filterProperty
  }

  return storage.NewArticlePage(pfc.C, filter, r.FormValue("continue"))
}

func createFolder(pfc *PFContext) (interface{}, error) {
  r := pfc.R

  title := r.PostFormValue("folderName")
  if title == "" {
    return nil, NewReadableError(_l("Missing folder name"), nil)
  }

  if utf8.RuneCountInString(title) > 200 {
    return nil, NewReadableError(_l("Folder name is too long"), nil)
  }

  if exists, err := storage.IsFolderDuplicate(pfc.C, pfc.UserID, title); err != nil {
    return nil, err
  } else if exists {
    return nil, NewReadableError(_l("A folder with that name already exists"), nil)
  }

  if _, err := storage.CreateFolder(pfc.C, pfc.UserID, title); err != nil {
    return nil, NewReadableError(_l("An error occurred while adding the new folder"), &err)
  }

  return storage.NewUserSubscriptions(pfc.C, pfc.UserID)
}

func rename(pfc *PFContext) (interface{}, error) {
  r := pfc.R

  title := r.PostFormValue("title")
  if title == "" {
    return nil, NewReadableError(_l("Name not specified"), nil)
  }

  folderID := r.PostFormValue("folder")

  if subscriptionID := r.PostFormValue("subscription"); subscriptionID != "" {
    // Rename subscription
    ref := storage.SubscriptionRef {
      FolderRef: storage.FolderRef {
        UserID: pfc.UserID,
        FolderID: folderID,
      },
      SubscriptionID: subscriptionID,
    }
    if err := storage.RenameSubscription(pfc.C, ref, title); err != nil {
      return nil, NewReadableError(_l("Error renaming folder"), &err)
    }
  } else if folderID != "" {
    // Rename folder
    if exists, err := storage.IsFolderDuplicate(pfc.C, pfc.UserID, title); err != nil {
      return nil, err
    } else if exists {
      return nil, NewReadableError(_l("A folder with that name already exists"), nil)
    }

    ref := storage.FolderRef {
      UserID: pfc.UserID,
      FolderID: folderID,
    }

    if err := storage.RenameFolder(pfc.C, ref, title); err != nil {
      return nil, NewReadableError(_l("Error renaming folder"), &err)
    }
  } else {
    return nil, NewReadableError(_l("Nothing to rename"), nil)
  }

  return storage.NewUserSubscriptions(pfc.C, pfc.UserID)
}

func setProperty(pfc *PFContext) (interface{}, error) {
  r := pfc.R

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
        UserID: pfc.UserID,
        FolderID: folderID,
      },
      SubscriptionID: subscriptionID,
    },
    ArticleID: articleID,
  }

  if properties, err := storage.SetProperty(pfc.C, ref, propertyName, propertyValue); err != nil {
    return nil, NewReadableError(_l("Error updating article"), &err)
  } else {
    return properties, nil
  }
}

func subscribe(pfc *PFContext) (interface{}, error) {
  c := pfc.C
  r := pfc.R

  subscriptionURL := r.PostFormValue("url")
  folderId := r.PostFormValue("folder")

  if subscriptionURL == "" {
    return nil, NewReadableError(_l("Missing URL"), nil)
  } else if _, err := url.ParseRequestURI(subscriptionURL); err != nil {
    return nil, NewReadableError(_l("URL is not valid"), &err)
  }

  if folderId != "" {
    ref := storage.FolderRef {
      UserID: pfc.UserID,
      FolderID: folderId,
    }

    if exists, err := storage.FolderExists(pfc.C, ref); err != nil {
      return nil, err
    } else if !exists {
      return nil, NewReadableError(_l("Folder not found"), nil)
    }
  }

  if exists, err := storage.IsFeedAvailable(pfc.C, subscriptionURL); err != nil {
    return nil, err
  } else if !exists {
    // Not a known feed URL
    // Match it against a list of known WWW links

    if feedURL, err := storage.WebToFeedURL(pfc.C, subscriptionURL); err != nil {
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

      if feedURL, err := storage.WebToFeedURL(pfc.C, modifiedURL); err != nil {
        return nil, err
      } else if feedURL != "" {
        subscriptionURL = feedURL
      }
    }
  }

  if subscribed, err := storage.IsSubscriptionDuplicate(pfc.C, pfc.UserID, subscriptionURL); err != nil {
    return nil, err
  } else if subscribed {
    return nil, NewReadableError(_l("You are already subscribed"), nil)
  }

  // At this point, the URL may have been re-written, so we check again
  if exists, err := storage.IsFeedAvailable(pfc.C, subscriptionURL); err != nil {
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

        if linkURL, err := rss.ExtractRSSLink(c, subscriptionURL, body); linkURL == "" || err != nil {
          return nil, NewReadableError(_l("RSS content not found"), &err)
        } else {
          subscriptionURL = linkURL
        }
      }
    }
  }

  params := taskParams {
    "url":      subscriptionURL,
    "folderID": folderId,
  }
  if err := startTask(pfc, "subscribe", params, subscriptionQueue); err != nil {
    return nil, NewReadableError(_l("Cannot subscribe - too busy"), &err)
  }

  return _l("Subscribing, please wait…"), nil
}

func unsubscribe(pfc *PFContext) (interface{}, error) {
  r := pfc.R

  subscriptionID := r.PostFormValue("subscription")
  folderID := r.PostFormValue("folder")

  // Remove a subscription
  ref := storage.SubscriptionRef {
    FolderRef: storage.FolderRef {
      UserID: pfc.UserID,
      FolderID: folderID,
    },
    SubscriptionID: subscriptionID,
  }

  if exists, err := storage.SubscriptionExists(pfc.C, ref); err != nil {
    return nil, err
  } else if !exists {
    return nil, NewReadableError(_l("Subscription not found"), nil)
  }

  if err := storage.Unsubscribe(pfc.C, ref); err != nil {
    return nil, err
  }

  params := taskParams {
    "subscriptionID": subscriptionID,
    "folderID": folderID,
  }
  if err := startTask(pfc, "unsubscribe", params, modificationQueue); err != nil {
    return nil, NewReadableError(_l("Cannot unsubscribe - too busy"), &err)
  }

  return storage.NewUserSubscriptions(pfc.C, pfc.UserID)
}

func importOPML(pfc *PFContext) (interface{}, error) {
  c := pfc.C
  r := pfc.R

  blobs, other, err := blobstore.ParseUpload(r)
  if err != nil {
    return nil, NewReadableError(_l("Error receiving file"), &err)
  } else if len(other["client"]) > 0 {
    if clientID := other["client"][0]; clientID != "" {
      pfc.ChannelID = string(pfc.UserID) + "," + clientID
    }
  }

  var blobKey appengine.BlobKey
  if blobInfos := blobs["opml"]; len(blobInfos) == 0 {
    return nil, NewReadableError(_l("File not uploaded"), nil)
  } else {
    blobKey = blobInfos[0].BlobKey
    reader := blobstore.NewReader(c, blobKey)

    var doc opml.Document
    if err := opml.Parse(reader, &doc); err != nil {
      if err := blobstore.Delete(c, blobKey); err != nil {
        c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
      }

      return nil, NewReadableError(_l("Error reading OPML file"), &err)
    }
  }

  params := taskParams {
    "opmlBlobKey": string(blobKey),
  }
  if err := startTask(pfc, "import", params, importQueue); err != nil {
    // Remove the blob
    if err := blobstore.Delete(c, blobKey); err != nil {
      c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
    }

    return nil, NewReadableError(_l("Cannot import - too busy"), &err)
  }

  return _l("Importing, please wait…"), nil
}

func markAllAsRead(pfc *PFContext) (interface{}, error) {
  r := pfc.R

  subscriptionID := r.PostFormValue("subscription")
  folderID := r.PostFormValue("folder")

  if subscriptionID != "" {
    ref := storage.SubscriptionRef {
      FolderRef: storage.FolderRef {
        UserID: pfc.UserID,
        FolderID: folderID,
      },
      SubscriptionID: subscriptionID,
    }
    if exists, err := storage.SubscriptionExists(pfc.C, ref); err != nil {
      return nil, err
    } else if !exists {
      return nil, NewReadableError(_l("Subscription not found"), nil)
    }
  } else if folderID != "" {
    ref := storage.FolderRef {
      UserID: pfc.UserID,
      FolderID: folderID,
    }

    if exists, err := storage.FolderExists(pfc.C, ref); err != nil {
      return nil, err
    } else if !exists {
      return nil, NewReadableError(_l("Folder not found"), nil)
    }
  }

  params := taskParams {
    "subscriptionID": subscriptionID,
    "folderID":       folderID,
  }
  if err := startTask(pfc, "markAllAsRead", params, modificationQueue); err != nil {
    return nil, err
  }

  return _l("Please wait…"), nil
}

func moveSubscription(pfc *PFContext) (interface{}, error) {
  r := pfc.R

  subscriptionID := r.PostFormValue("subscription")
  folderID := r.PostFormValue("folder")
  destinationID := r.PostFormValue("destination")
  
  if destinationID != "" {
    destination := storage.FolderRef {
      UserID: pfc.UserID,
      FolderID: destinationID,
    }
    if exists, err := storage.FolderExists(pfc.C, destination); err != nil {
      return nil, err
    } else if !exists {
      return nil, NewReadableError(_l("Folder not found"), nil)
    }
  }

  ref := storage.SubscriptionRef {
    FolderRef: storage.FolderRef {
      UserID: pfc.UserID,
      FolderID: folderID,
    },
    SubscriptionID: subscriptionID,
  }

  if exists, err := storage.SubscriptionExists(pfc.C, ref); err != nil {
    return nil, err
  } else if !exists {
    return nil, NewReadableError(_l("Subscription not found"), nil)
  }

  params := taskParams {
    "subscriptionID": subscriptionID,
    "folderID":       folderID,
    "destinationID":  destinationID,
  }

  if err := startTask(pfc, "moveSubscription", params, modificationQueue); err != nil {
    return nil, err
  }

  return _l("Please wait…"), nil
}

func authUpload(pfc *PFContext) (interface{}, error) {
  c := pfc.C

  if uploadURL, err := blobstore.UploadURL(c, "/import", nil); err != nil {
    return nil, err
  } else {
    return map[string]string { "uploadUrl": uploadURL.String() }, nil
  }
}

func initChannel(pfc *PFContext) (interface{}, error) {
  if pfc.ChannelID == "" {
    return nil, NewReadableError(_l("Missing Client ID"), nil)
  }

  if token, err := channel.Create(pfc.C, pfc.ChannelID); err != nil {
    return nil, NewReadableError(_l("Error initializing channel"), &err)
  } else {
    return map[string]string { "token": token }, nil
  }
}

func removeFolder(pfc *PFContext) (interface{}, error) {
  r := pfc.R

  folderID := r.PostFormValue("folder")
  if folderID == "" {
    return nil, NewReadableError(_l("Folder not found"), nil)
  }

  folderRef := storage.FolderRef {
    UserID: pfc.UserID,
    FolderID: folderID,
  }

  if exists, err := storage.FolderExists(pfc.C, folderRef); err != nil {
    return nil, err
  } else if !exists {
    return nil, NewReadableError(_l("Folder not found"), nil)
  }

  params := taskParams {
    "folderID": folderID,
  }
  if err := startTask(pfc, "removeFolder", params, modificationQueue); err != nil {
    return nil, NewReadableError(_l("Cannot unsubscribe - too busy"), &err)
  }

  return _l("Please wait…"), nil
}
