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
 "appengine/urlfetch"
 "rss"
 "storage"
 "time"
)

func registerCron() {
  RegisterCronRoute("/cron/updateFeeds", updateFeedsJob)
}

func updateFeed(c appengine.Context, ch chan<- storage.Feed, feed storage.Feed) {
  client := urlfetch.Client(c)
  if response, err := client.Get(feed.URL); err != nil {
    c.Errorf("Error downloading feed %s: %s", feed.URL, err)
    goto done
  } else {
    defer response.Body.Close()
    if parsedFeed, err := rss.UnmarshalStream(feed.URL, response.Body); err != nil {
      c.Errorf("Error reading RSS content: %s", err)
      goto done
    } else if err := storage.UpdateFeed(c, parsedFeed); err != nil {
      c.Errorf("Error updating feed: %s", err)
      goto done
    }
  }

done:
  ch<- feed
}

func updateFeedsJob(pfc *PFContext) error {
  c := pfc.C
  feed := storage.Feed{}
  importing := 0
  importStarted := time.Now()
  doneChannel := make(chan storage.Feed)

  q := datastore.NewQuery("Feed").Filter("NextFetch <", time.Now())
  for t := q.Run(c); ; {
    if _, err := t.Next(&feed); err == datastore.Done {
      break
    } else if err != nil {
      c.Errorf("Error fetching feed record: %s", err)
      return err
    }

    c.Infof("%s, overdue %s for update", feed.Title, time.Since(feed.Fetched))

    go updateFeed(pfc.C, doneChannel, feed)
    importing++
  }

  for i := 0; i < importing; i++ {
    feed := <-doneChannel;
    c.Infof("Completed feed %s", feed.Title)
  }

  c.Infof("%d feeds completed in %s", importing, time.Since(importStarted))

  return nil
}
