/*****************************************************************************
 **
 ** PerFeediem
 ** https://github.com/melllvar/PerFeediem
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
  "appengine/datastore"
  "html"
  "rss"
  "regexp"
  "sanitize"
  "time"
)

type PageMarker struct {
  SessionID string
  Cursor string
}

type Feed struct {
  URL string
  Title string
  Description string `datastore:",noindex"`
  Updated time.Time
  Link string
  Format string
  Retrieved time.Time
  HourlyUpdateFrequency float32

  EntryMetas []*EntryMeta `datastore:"-"`
  Entries []*Entry        `datastore:"-"`
}

type EntryMeta struct {
  Published time.Time
  Retrieved time.Time
  Entry *datastore.Key
}

type Entry struct {
  UniqueID string     `json:"-"`
  Author string       `json:"author"`
  Title string        `json:"title"`
  Link string         `json:"link"`
  Published time.Time `json:"published"`
  Updated time.Time   `json:"-"`

  Content string `json:"content" datastore:",noindex"`
  Summary string `json:"summary"`
}

type Subscription struct {
  ID string    `datastore:"-" json:"id"`
  Link string  `datastore:"-" json:"link"`

  Updated time.Time    `json:"-"`
  Subscribed time.Time `json:"-"`
  Feed *datastore.Key  `json:"-"`

  Title string         `json:"title"`
  UnreadCount int      `json:"unread"`
}

type SubEntry struct {
  ID string             `datastore:"-" json:"id"`
  Source string         `datastore:"-" json:"source"`
  Details *Entry        `datastore:"-" json:"details"`

  Published time.Time   `json:"-"`
  Retrieved time.Time   `json:"-"`
  Entry *datastore.Key  `json:"-"`

  Properties []string   `json:"properties"`
}

type UserSubscriptions struct {
  Subscriptions []*Subscription `json:"subscriptions"`
  Folders []*SubFolder          `json:"folders"`
}

type SubFolder struct {
  ID string    `datastore:"-" json:"id"`
  Title string `json:"title"`
}

var extraSpaceStripper *regexp.Regexp = regexp.MustCompile(`\s\s+`)

func generateSummary(entry *rss.Entry) string {
  // TODO: This process should be streamlined to do
  // more things with fewer passes
  sanitized := sanitize.StripTags(entry.Content)
  unescaped := html.UnescapeString(sanitized)
  stripped := extraSpaceStripper.ReplaceAllString(unescaped, "")

  if runes := []rune(stripped); len(runes) > 400 {
    return string(runes[:400])
  } else {
    return stripped
  }
}

func NewFeed(parsedFeed *rss.Feed) (*Feed, error) {
  feed := Feed {
    URL: parsedFeed.URL,
    Title: parsedFeed.Title,
    Description: parsedFeed.Description,
    Updated: parsedFeed.Updated,
    Link: parsedFeed.WWWURL,
    Format: parsedFeed.Format,
    Retrieved: parsedFeed.Retrieved,
    HourlyUpdateFrequency: parsedFeed.HourlyUpdateFrequency,
  }

  feed.Entries = make([]*Entry, len(parsedFeed.Entries))
  feed.EntryMetas = make([]*EntryMeta, len(parsedFeed.Entries))

  for i, parsedEntry := range parsedFeed.Entries {
    feed.Entries[i] = &Entry {
      UniqueID: parsedEntry.UniqueID(),
      Author: html.UnescapeString(parsedEntry.Author),
      Title: html.UnescapeString(parsedEntry.Title),
      Link: parsedEntry.WWWURL,
      Published: parsedEntry.Published,
      Updated: parsedEntry.Updated,
      Content: parsedEntry.Content,
      Summary: generateSummary(parsedEntry),
    }
    feed.EntryMetas[i] = &EntryMeta {
      Published: parsedEntry.Published,
      Retrieved: parsedFeed.Retrieved,
    }
  }

  return &feed, nil
}
