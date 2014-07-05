/*****************************************************************************
 **
 ** Gofr
 ** https://github.com/pokebyte/Gofr
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
	"errors"
	"net/url"
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
	RegisterTaskRoute("/tasks/moveSubscription", moveSubscriptionTask)
	RegisterTaskRoute("/tasks/syncFeeds",     syncFeedsTask)
	RegisterTaskRoute("/tasks/removeFolder",  removeFolderTask)
	RegisterTaskRoute("/tasks/removeTag",     removeTagTask)
}

func startTask(pfc *PFContext, taskName string, params taskParams, queueName string) error {
	taskValues := url.Values {
		"userID": { pfc.User.ID },
		"channelID": { pfc.ChannelID },
	}

	for k, v := range params {
		taskValues.Add(k, v)
	}

	task := taskqueue.NewPOSTTask("/tasks/" + taskName, taskValues)
	if _, err := taskqueue.Add(pfc.C, task, queueName); err != nil {
		return err
	}

	return nil
}

func importSubscription(pfc *PFContext, ch chan<- *rss.Outline, userID storage.UserID, folderRef storage.FolderRef, outline *rss.Outline) {
	c := pfc.C
	subscriptionURL := outline.FeedURL

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
		client := createHttpClient(pfc.C)
		if response, err := client.Get(subscriptionURL); err != nil {
			c.Errorf("Error downloading feed %s: %s", subscriptionURL, err)
			goto done
		} else {
			defer response.Body.Close()
			if parsedFeed, err := rss.UnmarshalStream(subscriptionURL, response.Body); err != nil {
				c.Errorf("Error reading RSS content (%s): %s", subscriptionURL, err)
				goto done
			} else {
				favIconURL := ""
				if parsedFeed.WWWURL != "" {
					if url, err := locateFavIconURL(pfc.C, parsedFeed.WWWURL); err != nil {
						// Not critical
						pfc.C.Warningf("FavIcon retrieval error: %s", err)
					} else if url != "" {
						favIconURL = url
					}
				}

				if err := storage.UpdateFeed(pfc.C, parsedFeed, favIconURL, time.Now()); err != nil {
					c.Errorf("Error updating feed: %s", err)
					goto done
				}
			}
		}
	}

	if subscriptionRef, err := storage.Subscribe(pfc.C, folderRef, subscriptionURL, outline.Title); err != nil {
		c.Errorf("Error subscribing to feed %s: %s", subscriptionURL, err)
		goto done
	} else {
		if _, err := storage.UpdateSubscription(pfc.C, subscriptionURL, subscriptionRef); err != nil {
			c.Errorf("Error updating subscription %s: %s", subscriptionURL, err)
			goto done
		}
	}

done:
	ch<- outline
}

func importSubscriptions(pfc *PFContext, ch chan<- *rss.Outline, userID storage.UserID, parentRef storage.FolderRef, outlines []*rss.Outline) int {
	c := pfc.C

	count := 0
	for _, outline := range outlines {
		if outline.IsSubscription() {
			go importSubscription(pfc, ch, userID, parentRef, outline)
			count++
		} else if outline.IsFolder() {
			folderRef, err := storage.FolderByTitle(pfc.C, userID, outline.Title)
			if err != nil {
				c.Warningf("Error locating folder: %s", err)
				continue
			} else if folderRef.IsZero() {
				if folderRef, err = storage.CreateFolder(pfc.C, userID, outline.Title); err != nil {
					c.Warningf("Error locating folder: %s", err)
					continue
				}
			}

			count += importSubscriptions(pfc, ch, userID, folderRef, outline.Outlines)
		}
	}

	return count
}

func importOPMLTask(pfc *PFContext) (TaskMessage, error) {
	c := pfc.C

	var blobKey appengine.BlobKey
	if blobKeyString := pfc.R.PostFormValue("opmlBlobKey"); blobKeyString == "" {
		return TaskMessage{}, errors.New("Missing blob key")
	} else {
		blobKey = appengine.BlobKey(blobKeyString)
	}

	reader := blobstore.NewReader(c, blobKey)

	opml, err := rss.ParseOPML(reader)
	if err != nil {
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
		UserID: pfc.UserID,
	}

	doneChannel := make(chan *rss.Outline)
	importing := importSubscriptions(pfc, doneChannel, pfc.UserID, parentRef, opml.Outlines())

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
	subscriptionURL := pfc.R.PostFormValue("url")
	folderID := pfc.R.PostFormValue("folderID")

	if subscriptionURL == "" {
		return TaskMessage{}, errors.New("Missing subscription URL")
	}

	subscriptionRef := storage.SubscriptionRef {
		FolderRef: storage.FolderRef {
			UserID: pfc.UserID,
			FolderID: folderID,
		},
		SubscriptionID: subscriptionURL,
	}

	if exists, err := storage.SubscriptionExists(pfc.C, subscriptionRef); err != nil {
		return TaskMessage{}, err
	} else if !exists {
		pfc.C.Warningf("No longer subscribed to %s", subscriptionURL, err)
		return TaskMessage{}, nil
	}

	if feed, err := storage.FeedByURL(pfc.C, subscriptionURL); err != nil {
		return TaskMessage{}, err
	} else if feed == nil {
		// Feed not available locally - fetch it
		client := createHttpClient(pfc.C)
		if response, err := client.Get(subscriptionURL); err != nil {
			pfc.C.Errorf("Error downloading feed (%s): %s", subscriptionURL, err)
			return TaskMessage{}, NewReadableError(_l("An error occurred while downloading the feed"), &err)
		} else {
			defer response.Body.Close()
			if parsedFeed, err := rss.UnmarshalStream(subscriptionURL, response.Body); err != nil {
				pfc.C.Errorf("Error reading RSS content (%s): %s", subscriptionURL, err)
				return TaskMessage{}, NewReadableError(_l("Error reading RSS content"), &err)
			} else {
				favIconURL := ""
				if parsedFeed.WWWURL != "" {
					if url, err := locateFavIconURL(pfc.C, parsedFeed.WWWURL); err != nil {
						// Not critical
						pfc.C.Warningf("FavIcon retrieval error: %s", err)
					} else if url != "" {
						favIconURL = url
					}
				}

				if err := storage.UpdateFeed(pfc.C, parsedFeed, favIconURL, time.Now()); err != nil {
					return TaskMessage{}, err
				}
			}
		}
	}

	if _, err := storage.UpdateSubscription(pfc.C, subscriptionURL, subscriptionRef); err != nil {
		return TaskMessage{}, err
	}

	return TaskMessage{
		Refresh: true,
	}, nil
}

func unsubscribeTask(pfc *PFContext) (TaskMessage, error) {
	folderID := pfc.R.PostFormValue("folderID")
	subscriptionID := pfc.R.PostFormValue("subscriptionID")

	if subscriptionID == "" {
		return TaskMessage{}, errors.New("Missing subscription ID")
	}

	ref := storage.ArticleScope {
		FolderRef: storage.FolderRef {
			UserID: pfc.UserID,
			FolderID: folderID,
		},
		SubscriptionID: subscriptionID,
	}

	if err := storage.DeleteArticlesWithinScope(pfc.C, ref); err != nil {
		return TaskMessage{}, err
	}

	return TaskMessage{
		Refresh: false,
	}, nil
}

func markAllAsReadTask(pfc *PFContext) (TaskMessage, error) {
	folderID := pfc.R.PostFormValue("folderID")
	subscriptionID := pfc.R.PostFormValue("subscriptionID")

	ref := storage.ArticleScope {
		FolderRef: storage.FolderRef {
			UserID: pfc.UserID,
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

func moveSubscriptionTask(pfc *PFContext) (TaskMessage, error) {
	r := pfc.R

	subscriptionID := r.PostFormValue("subscriptionID")
	folderID := r.PostFormValue("folderID")
	destinationID := r.PostFormValue("destinationID")
	
	subscription := storage.SubscriptionRef {
		FolderRef: storage.FolderRef {
			UserID: pfc.UserID,
			FolderID: folderID,
		},
		SubscriptionID: subscriptionID,
	}

	destination := storage.FolderRef {
		UserID: pfc.UserID,
		FolderID: destinationID,
	}

	if err := storage.MoveArticles(pfc.C, subscription, destination); err != nil {
		return TaskMessage{}, err
	}
	
	return TaskMessage{}, nil
}

func syncFeedsTask(pfc *PFContext) (TaskMessage, error) {
	if err := storage.UpdateAllSubscriptions(pfc.C, pfc.UserID); err != nil {
		return TaskMessage{}, err
	}

	userSubscriptions, err := storage.NewUserSubscriptions(pfc.C, pfc.UserID)
	if err != nil {
		return TaskMessage{}, err
	}

	return TaskMessage {
		Refresh: false,
		Subscriptions: userSubscriptions,
	}, nil
}

func removeFolderTask(pfc *PFContext) (TaskMessage, error) {
	folderID := pfc.R.PostFormValue("folderID")
	ref := storage.ArticleScope {
		FolderRef: storage.FolderRef {
			UserID: pfc.UserID,
			FolderID: folderID,
		},
	}

	if err := storage.DeleteArticlesWithinScope(pfc.C, ref); err != nil {
		return TaskMessage{}, err
	}

	return TaskMessage{}, nil
}

func removeTagTask(pfc *PFContext) (TaskMessage, error) {
	tagID := pfc.R.PostFormValue("tagID")
	if err := storage.RemoveTag(pfc.C, pfc.UserID, tagID); err != nil {
		return TaskMessage{}, err
	}

	return TaskMessage{}, nil
}
