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
	"time"
)

type Feed_V1 struct {
	URL string
	Title string
	Description string `datastore:",noindex"`
	Updated time.Time
	Link string
	Format string
	Fetched time.Time
	NextFetch time.Time
	HourlyUpdateFrequency float32
	UpdateCounter int64
	Topic string
	HubURL string
}

func Migrate(c appengine.Context, currentVersion int, finalVersion int) (int, error) {
	var version int
	for version = currentVersion + 1; version <= finalVersion; version++ {
		c.Infof("Migrating to version %d...", version)
		if version == 2 {
			if err := migrateToVersion2(c); err != nil {
				return version, err
			}
		}
	}

	c.Infof("All done!")

	return finalVersion, nil
}

func migrateToVersion2(c appengine.Context) error {
	batchWriter := NewBatchWriterWithSize(c, BatchPut, 100)

	c.Infof("Migrating Feeds...")

	q := datastore.NewQuery("Feed")
	for t := q.Run(c); ; {
		feedV1 := new(Feed_V1)
		feedV1Key, err := t.Next(feedV1)

		if err == datastore.Done {
			break
		} else if err != nil {
			return err
		}

		feed := Feed{
			URL: feedV1.URL,
			Title: feedV1.Title,
			Description: feedV1.Description,
			Link: feedV1.Link,
			Format: feedV1.Format,
			Topic: feedV1.Topic,
			HubURL: feedV1.HubURL,
			Updated: feedV1.Updated,
		}

		feedMeta := FeedMeta {
			Feed: feedV1Key,
			InfoDigest: []byte{},
			Fetched: feedV1.Fetched,
			NextFetch: feedV1.NextFetch,
			UpdateCounter: feedV1.UpdateCounter,
			HourlyUpdateFrequency: feedV1.HourlyUpdateFrequency,
			Updated: feedV1.Updated,
		}

		feedKey := datastore.NewKey(c, "Feed", feedV1Key.StringID(), 0, nil)
		feedMetaKey := datastore.NewKey(c, "FeedMeta", feedV1Key.StringID(), 0, nil)

		if err := batchWriter.Enqueue(feedKey, &feed); err != nil {
			c.Errorf("Error queueing Feed for batch write: %s", err)
			return err
		}

		if err := batchWriter.Enqueue(feedMetaKey, &feedMeta); err != nil {
			c.Errorf("Error queueing FeedMeta for batch write: %s", err)
			return err
		}
	}

	if err := batchWriter.Flush(); err != nil {
		c.Errorf("Error flushing Feed/FeedMeta batch queue: %s", err)
		return err
	}

	return nil
}
