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
	"appengine/datastore"
	"rss"
	"storage"
	"strings"
	"time"
)

func registerCron() {
	RegisterCronRoute("/cron/updateFeeds", updateFeedsJob)
	RegisterCronRoute("/cron/updateUnreadCounts", updateUnreadCountsJob)
	RegisterCronRoute("/cron/updateFavIcons", updateFavIconsJob)
}

func updateFeed(c appengine.Context, ch chan<- *storage.FeedMeta, url string, feedMeta *storage.FeedMeta) {
	client := createHttpClient(c)
	if response, err := client.Get(url); err != nil {
		c.Errorf("Error downloading feed %s: %s", url, err)
		goto done
	} else {
		defer response.Body.Close()
		if parsedFeed, err := rss.UnmarshalStream(url, response.Body); err != nil {
			c.Errorf("Error reading RSS content (%s): %s", url, err)
			goto done
		} else if err := storage.UpdateFeed(c, parsedFeed, "", time.Now()); err != nil {
			c.Errorf("Error updating feed: %s", err)
			goto done
		}
	}

done:
	ch<- feedMeta
}

func updateFeedsJob(pfc *PFContext) error {
	c := pfc.C
	importing := 0
	started := time.Now()
	fetchTime := time.Now()
	doneChannel := make(chan *storage.FeedMeta)
	var jobError error

	if appengine.IsDevAppServer() {
		// On dev server, disregard next update limitations 
		// (by "forwarding the clock")
		fetchTime = fetchTime.Add(time.Duration(24) * time.Hour)
	}
	
	q := datastore.NewQuery("FeedMeta").Filter("NextFetch <", fetchTime)
	for t := q.Run(c); ; {
		feedMeta := new(storage.FeedMeta)
		var feedMetaKey *datastore.Key

		if key, err := t.Next(feedMeta); err == datastore.Done {
			break
		} else if err == nil || storage.IsFieldMismatch(err) {
			feedMetaKey = key
		} else {
			c.Errorf("Error fetching feed record: %s", err)
			jobError = err
			break
		}

		go updateFeed(pfc.C, doneChannel, feedMetaKey.StringID(), feedMeta)
		importing++
	}

	for i := 0; i < importing; i++ {
		<-doneChannel;
	}

	c.Infof("%d feeds completed in %s", importing, time.Since(started))

	return jobError
}

func updateUnreadCountsJob(pfc *PFContext) error {
	c := pfc.C
	routines := 0
	started := time.Now()
	doneChannel := make(chan storage.Subscription)
	subscription := storage.Subscription{}
	var jobError error

	q := datastore.NewQuery("Subscription")
	for t := q.Run(c); ; {
		subscriptionKey, err := t.Next(&subscription)
		if err == datastore.Done {
			break
		} else if err != nil {
			c.Errorf("Error fetching subscription: %s", err)
			jobError = err
			break
		}

		go storage.UpdateUnreadCounts(pfc.C, doneChannel, subscriptionKey, subscription)
		routines++
	}

	for i := 0; i < routines; i++ {
		<-doneChannel;
	}

	c.Infof("%d subscriptions scanned in %s", routines, time.Since(started))

	return jobError
}

func updateFavIcon(c appengine.Context, ch chan<- *storage.Feed, feedKey *datastore.Key, feed *storage.Feed) {
	if favIconURL, err := locateFavIconURL(c, feed.Link); err != nil {
		// Not critical
		c.Warningf("FavIcon retrieval error: %s", err)
	} else if favIconURL != "" {
		feed.FavIconURL = favIconURL
		if _, err := datastore.Put(c, feedKey, feed); err != nil {
			c.Warningf("FavIcon write error: %s", err)
		}
	}

	ch<- feed
}

func updateFavIconsJob(pfc *PFContext) error {
	var jobError error
	c := pfc.C
	doneChannel := make(chan *storage.Feed)
	started := time.Now()
	count, routines, skipped := 0, 0, 0
	fullRefresh := strings.ToLower(pfc.R.PostFormValue("fullRefresh")) == "true"

	q := datastore.NewQuery("Feed")
	for t := q.Run(c); ; {
		feed := new(storage.Feed)

		if feedKey, err := t.Next(feed); err == datastore.Done {
			break
		} else if err == nil || storage.IsFieldMismatch(err) {
			if feed.FavIconURL != "" && !fullRefresh {
				// FavIcon already present
				skipped++
			} else if feed.Link == "" {
				// No link to the home page
				skipped++
			} else {
				routines++
				go updateFavIcon(c, doneChannel, feedKey, feed)
			}
		} else {
			c.Warningf("Error fetching feed record: %s", err)
			jobError = err
			break
		}

		count++
	}

	for i := 0; i < routines; i++ {
		<-doneChannel;
	}

 	c.Infof("%d feeds completed (%d skipped) in %s", count, skipped, time.Since(started))

	return jobError
}

