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
 
package storage

import (
	"appengine"
	"appengine/datastore"
	"errors"
	"strconv"
	"strings"
	"time"
)

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

	return "", 0, errors.New("Missing valid identifier")
}

func newFolderRef(userID UserID, key *datastore.Key) (FolderRef) {
	ref := FolderRef {
		UserID: userID,
	}

	if key != nil {
		ref.FolderID = formatId("folder", key.IntID())
	}

	return ref
}

func updateSubscriptionByKey(c appengine.Context, subscriptionKey *datastore.Key, subscription Subscription) (int, error) {
	feedKey := subscription.Feed
	largestUpdateIndexWritten := int64(-1)
	unreadDelta := 0

	batchWriter := NewBatchWriter(c, BatchPut)

	q := datastore.NewQuery("EntryMeta").Ancestor(feedKey).Filter("UpdateIndex >", subscription.MaxUpdateIndex)
	for t := q.Run(c); ; {
		entryMeta := new(EntryMeta)
		_, err := t.Next(entryMeta)

		if err == datastore.Done {
			break
		} else if IsFieldMismatch(err) {
			// Ignore
		} else if err != nil {
			c.Errorf("Error reading Entry: %s", err)
			return batchWriter.Written(), err
		}

		articleKey := datastore.NewKey(c, "Article", entryMeta.Entry.StringID(), 0, subscriptionKey)
		article := Article{}

		if err := datastore.Get(c, articleKey, &article); err == datastore.ErrNoSuchEntity {
			// New article
			article.Entry = entryMeta.Entry
			article.Properties = []string { "unread" }
			unreadDelta++
		} else if IsFieldMismatch(err) {
			// Ignore - migration
		} else if err != nil {
			c.Warningf("Error reading article %s: %s", entryMeta.Entry.StringID(), err)
			continue
		}

		article.UpdateIndex = entryMeta.UpdateIndex
		article.Fetched = entryMeta.Fetched
		article.Published = entryMeta.Published

		if entryMeta.UpdateIndex > largestUpdateIndexWritten {
			largestUpdateIndexWritten = entryMeta.UpdateIndex
		}

		if err := batchWriter.Enqueue(articleKey, &article); err != nil {
			c.Errorf("Error queueing article for batch write: %s", err)
			return batchWriter.Written(), err
		}
	}

	if err := batchWriter.Flush(); err != nil {
		c.Errorf("Error flushing batch queue: %s", err)
		return batchWriter.Written(), err
	}

	if batchWriter.Written() > 0 {
		if appengine.IsDevAppServer() {
			c.Debugf("Completed %s: %d records", subscriptionKey.StringID(), batchWriter.Written())
		}

		// Write the subscription
		subscription.Updated = time.Now()
		subscription.MaxUpdateIndex = largestUpdateIndexWritten

		if subscription.UnreadCount + unreadDelta >= 0 {
			subscription.UnreadCount += unreadDelta
		}

		if _, err := datastore.Put(c, subscriptionKey, &subscription); err != nil {
			c.Errorf("Error writing subscription: %s", err)
			return batchWriter.Written(), err
		}

		// Update usage index (rough way to track feed popularity)
		// No sharding, no transactions - complete accuracy is unimportant for now

		feedUsage := FeedUsage{}
		feedUsageKey := datastore.NewKey(c, "FeedUsage", feedKey.StringID(), 0, nil)

		if err := datastore.Get(c, feedUsageKey, &feedUsage); err == datastore.ErrNoSuchEntity || err == nil {
			if err == datastore.ErrNoSuchEntity {
				// Create a new entity
				feedUsage.Feed = feedKey
			}

			feedUsage.UpdateCount++
			feedUsage.LastSubscriptionUpdate = time.Now()

			if _, err := datastore.Put(c, feedUsageKey, &feedUsage); err != nil {
				c.Warningf("Non-critical error updating feed usage (%s): %s", feedKey.StringID(), err)
			}
		}
	}

	return batchWriter.Written(), nil
}

func updateSubscriptionAsync(c appengine.Context, subscriptionKey *datastore.Key, subscription Subscription, ch chan<- Subscription) {
	if _, err := updateSubscriptionByKey(c, subscriptionKey, subscription); err != nil {
		c.Errorf("Error updating subscription %s: %s", subscription.Title, err)
	}

	ch <- subscription
}
