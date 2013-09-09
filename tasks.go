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
  "appengine/taskqueue"
  "appengine/urlfetch"
  "errors"
  "net/url"
  "opml"
  "rss"
  "storage"
  "time"
)

type taskParams map[string]string

func registerTasks() {
  RegisterTaskRoute("/tasks/subscribe",     subscribeTask)
  RegisterTaskRoute("/tasks/import",        importOPMLTask)
  RegisterTaskRoute("/tasks/unsubscribe",   unsubscribeTask)
  RegisterTaskRoute("/tasks/markAllAsRead", markAllAsReadTask)
}

func importSubscription(pfc *PFContext, ch chan<- *opml.Subscription, userID storage.UserID, folderRef storage.FolderRef, opmlSubscription *opml.Subscription) {
  c := pfc.C
  subscriptionURL := opmlSubscription.URL

  if subscribed, err := storage.IsSubscriptionDuplicate(pfc.C, userID, subscriptionURL); err != nil {
    c.Errorf("Cannot determine if '%s' is duplicate: %s", subscriptionURL, err)
    goto done
  } else if subscribed {
    c.Infof("Already subscribed to %s", subscriptionURL)
    goto done // Already subscribed
  }

  if feed, err := storage.FeedByURL(pfc.C, subscriptionURL); err != nil {
    c.Errorf("Error locating feed %s: %s", subscriptionURL, err.Error())
    goto done
  } else if feed == nil {
    // Feed not available locally - fetch it
    client := urlfetch.Client(pfc.C)
    if response, err := client.Get(subscriptionURL); err != nil {
      c.Errorf("Error downloading feed %s: %s", subscriptionURL, err)
      goto done
    } else {
      defer response.Body.Close()
      if parsedFeed, err := rss.UnmarshalStream(subscriptionURL, response.Body); err != nil {
        c.Errorf("Error reading RSS content: %s", err)
        goto done
      } else if err := storage.UpdateFeed(pfc.C, parsedFeed); err != nil {
        c.Errorf("Error updating feed: %s", err)
        goto done
      }
    }
  }

  if subscriptionRef, err := storage.Subscribe(pfc.C, folderRef, subscriptionURL, opmlSubscription.Title); err != nil {
    c.Errorf("Error subscribing to feed %s: %s", subscriptionURL, err)
    goto done
  } else {
    if err := storage.UpdateSubscription(pfc.C, subscriptionURL, subscriptionRef); err != nil {
      c.Errorf("Error updating subscription %s: %s", subscriptionURL, err)
      goto done
    }
  }

done:
  ch<- opmlSubscription
}

func importSubscriptions(pfc *PFContext, ch chan<- *opml.Subscription, userID storage.UserID, parentRef storage.FolderRef, subscriptions []*opml.Subscription) int {
  c := pfc.C

  count := 0
  for _, subscription := range subscriptions {
    if subscription.URL != "" {
      go importSubscription(pfc, ch, userID, parentRef, subscription)
      count++
    }

    if subscription.Subscriptions != nil {
      folderRef, err := storage.FolderByTitle(pfc.C, userID, subscription.Title)
      if err != nil {
        c.Warningf("Error locating folder: %s", err)
        continue
      } else if folderRef.IsZero() {
        if folderRef, err = storage.CreateFolder(pfc.C, userID, subscription.Title); err != nil {
          c.Warningf("Error locating folder: %s", err)
          continue
        }
      }

      count += importSubscriptions(pfc, ch, userID, folderRef, subscription.Subscriptions)
    }
  }

  return count
}

func importOPMLTask(pfc *PFContext) (TaskMessage, error) {
  c := pfc.C

  if pfc.R.PostFormValue("userID") == "" {
    return TaskMessage{}, errors.New("Missing User ID")
  }

  userID := storage.UserID(pfc.R.PostFormValue("userID"))

  var doc opml.Document
  var blobKey appengine.BlobKey
  if blobKeyString := pfc.R.PostFormValue("opmlBlobKey"); blobKeyString == "" {
    return TaskMessage{}, errors.New("Missing blob key")
  } else {
    blobKey = appengine.BlobKey(blobKeyString)
  }

  reader := blobstore.NewReader(c, blobKey)
  if err := opml.Parse(reader, &doc); err != nil {
    // Remove the blob
    if err := blobstore.Delete(c, blobKey); err != nil {
      c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
    }

    return TaskMessage{}, err
  }
  
  // Remove the blob
  if err := blobstore.Delete(c, blobKey); err != nil {
    c.Warningf("Error deleting blob (key %s): %s", blobKey, err)
  }

  importStarted := time.Now()

  parentRef := storage.FolderRef {
    UserID: userID,
  }

  doneChannel := make(chan *opml.Subscription)
  importing := importSubscriptions(pfc, doneChannel, userID, parentRef, doc.Subscriptions)

  for i := 0; i < importing; i++ {
    subscription := <-doneChannel;
    c.Infof("Completed %s", subscription.Title)
  }

  c.Infof("All completed in %s", time.Since(importStarted))

  return TaskMessage{
    Message: _l("Subscriptions imported successfully"),
    Refresh: true,
    }, nil
}

func subscribeTask(pfc *PFContext) (TaskMessage, error) {
  if pfc.R.PostFormValue("userID") == "" {
    return TaskMessage{}, errors.New("Missing User ID")
  }

  subscriptionURL := pfc.R.PostFormValue("url")
  userID := storage.UserID(pfc.R.PostFormValue("userID"))
  folderID := pfc.R.PostFormValue("folderID")

  if subscriptionURL == "" {
    return TaskMessage{}, errors.New("Missing subscription URL")
  }

  if subscribed, err := storage.IsSubscriptionDuplicate(pfc.C, userID, subscriptionURL); err != nil {
    return TaskMessage{}, err
  } else if subscribed {
    return TaskMessage{
      Message: _l("You are already subscribed"),
      Refresh: true,
    }, nil
  }

  folderRef := storage.FolderRef {
    UserID: userID,
    FolderID: folderID,
  }

  feedTitle := ""
  if feed, err := storage.FeedByURL(pfc.C, subscriptionURL); err != nil {
    return TaskMessage{}, err
  } else if feed == nil {
    // Feed not available locally - fetch it
    client := urlfetch.Client(pfc.C)
    if response, err := client.Get(subscriptionURL); err != nil {
      return TaskMessage{}, NewReadableError(_l("An error occurred while downloading the feed"), &err)
    } else {
      defer response.Body.Close()
      if parsedFeed, err := rss.UnmarshalStream(subscriptionURL, response.Body); err != nil {
        return TaskMessage{}, NewReadableError(_l("Error reading RSS content"), &err)
      } else if err := storage.UpdateFeed(pfc.C, parsedFeed); err != nil {
        return TaskMessage{}, err
      } else {
        feedTitle = parsedFeed.Title
      }
    }
  } else {
    feedTitle = feed.Title
  }

  if subscriptionRef, err := storage.Subscribe(pfc.C, folderRef, subscriptionURL, feedTitle); err != nil {
    return TaskMessage{}, err
  } else {
    if err := storage.UpdateSubscription(pfc.C, subscriptionURL, subscriptionRef); err != nil {
      return TaskMessage{}, err
    }
  }

  return TaskMessage{
    Message: _l("You are now subscribed to '%s'", feedTitle),
    Refresh: true,
    }, nil
}

func unsubscribeTask(pfc *PFContext) (TaskMessage, error) {
  userID := storage.UserID(pfc.R.PostFormValue("userID"))
  folderID := pfc.R.PostFormValue("folderID")
  subscriptionID := pfc.R.PostFormValue("subscriptionID")

  if userID == "" {
    return TaskMessage{}, errors.New("Missing user ID")
  }

  folderRef := storage.FolderRef {
    UserID: userID,
    FolderID: folderID,
  }

  if subscriptionID != "" {
    ref := storage.SubscriptionRef {
      FolderRef: folderRef,
      SubscriptionID: subscriptionID,
    }

    if err := storage.Unsubscribe(pfc.C, ref); err != nil {
      return TaskMessage{}, err
    }
  } else if folderID != "" {
    if err := storage.UnsubscribeAllInFolder(pfc.C, folderRef); err != nil {
      return TaskMessage{}, err
    }
  } else {
    return TaskMessage{}, errors.New("Missing subscription constraint")
  }

  return TaskMessage{
    Refresh: true,
    }, nil
}

func markAllAsReadTask(pfc *PFContext) (TaskMessage, error) {
  userID := storage.UserID(pfc.R.PostFormValue("userID"))
  folderID := pfc.R.PostFormValue("folderID")
  subscriptionID := pfc.R.PostFormValue("subscriptionID")

  if userID == "" {
    return TaskMessage{}, errors.New("Missing user ID")
  }

  ref := storage.SubscriptionRef {
    FolderRef: storage.FolderRef {
      UserID: userID,
      FolderID: folderID,
    },
    SubscriptionID: subscriptionID,
  }

  if marked, err := storage.MarkAllAsRead(pfc.C, ref); err != nil {
    return TaskMessage{}, err
  } else {
    return TaskMessage {
      Message: _l("%d items marked as read", marked),
      Refresh: true,
    }, nil
  }
}

func startTask(pfc *PFContext, taskName string, params taskParams) error {
  taskValues := url.Values {
    "userID": { pfc.User.ID },
    "channelID": { pfc.ChannelID() },
  }

  for k, v := range params {
    taskValues.Add(k, v)
  }

  task := taskqueue.NewPOSTTask("/tasks/" + taskName, taskValues)
  if _, err := taskqueue.Add(pfc.C, task, ""); err != nil {
    return err
  }

  return nil
}
