/*****************************************************************************
 **
 ** FRAE
 ** https://github.com/melllvar/frae
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
 
package frae

import (
  "time"
  "appengine/datastore"

  "parser"
)

// FIXME: get rid of parser.Feed and parser.Entry

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
  GUID string         `json:"-"`
  Author string       `json:"author"`
  Title string        `json:"title"`
  Link string         `json:"link"`
  Published time.Time `json:"published"`
  Updated time.Time   `json:"-"`

  Content string `datastore:",noindex" json:"content"`
  Summary string `datastore:",noindex" json:"summary"`
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

func NewFeed(parsedFeed *parser.Feed) (*Feed, error) {
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
      GUID: parsedEntry.GUID,
      Author: parsedEntry.Author,
      Title: parsedEntry.Title,
      Link: parsedEntry.WWWURL,
      Published: parsedEntry.Published,
      Updated: parsedEntry.Updated,
      Content: parsedEntry.Content,
    }
    feed.EntryMetas[i] = &EntryMeta {
      Published: parsedEntry.Published,
      Retrieved: parsedFeed.Retrieved,
    }
  }

  return &feed, nil
}
