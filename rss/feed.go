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
 
package rss

import (
  "time"
  "encoding/xml"
  "io"
  "io/ioutil"
  "bytes"
  "errors"
  "strings"
  "sort"
)

type Feed struct {
  URL string
  Title string
  Description string
  Updated time.Time
  WWWURL string
  Format string
  Retrieved time.Time
  HourlyUpdateFrequency float32
  Entries []*Entry
}

type Entry struct {
  GUID string
  Author string
  Title string
  WWWURL string
  Content string
  Published time.Time
  Updated time.Time
}

type SortableTimes []time.Time 

func (s SortableTimes) Len() int {
  return len(s)
}

func (s SortableTimes) Swap(i int, j int) {
  s[i], s[j] = s[j], s[i]
}

func (s SortableTimes) Less(i int, j int) bool {
  return s[i].Before(s[j])
}

func (feed *Feed)DurationBetweenUpdates() time.Duration {
  if feed.HourlyUpdateFrequency != 0 {
    // Set explicitly
    return time.Duration(feed.HourlyUpdateFrequency) * time.Hour
  }

  // Compute frequency by analyzing entries in the feed
  pubDates := make(SortableTimes, len(feed.Entries))
  for i, entry := range feed.Entries {
    pubDates[i] = entry.LatestModification()
  }

  // Sort dates in ascending order
  sort.Sort(pubDates)

  // Compute the average difference between them
  durationBetweenUpdates := time.Duration(0)
  if len(pubDates) > 1 {
    deltaSum := 0.0
    for i, n := 1, len(pubDates); i < n; i++ {
      deltaSum += float64(pubDates[i].Sub(pubDates[i - 1]).Hours())
    }

    durationBetweenUpdates = time.Duration(deltaSum / float64(len(pubDates) - 1)) * time.Hour
  }

  // Clamp the frequency
  minFrequency := time.Duration(30) * time.Minute // 30 minutes
  maxFrequency := time.Duration(24) * time.Hour   // 1 day

  if durationBetweenUpdates > maxFrequency {
    return maxFrequency
  } else if durationBetweenUpdates < minFrequency {
    return minFrequency
  }

  return durationBetweenUpdates
}

func (entry *Entry)LatestModification() time.Time {
  if entry.Updated.After(entry.Published) {
    return entry.Updated
  }

  return entry.Published
}

func (entry *Entry)UniqueID() string {
  if entry.GUID != "" {
    return entry.GUID
  }

  if !entry.LatestModification().IsZero() {
    return entry.WWWURL + "@" + entry.LatestModification().String()
  }

  return entry.WWWURL
}

type FeedMarshaler interface {
  Marshal() (Feed, error)
}

type GenericFeed struct {
  XMLName xml.Name
}

func charsetReader(charset string, r io.Reader) (io.Reader, error) {
  // FIXME: This hardly does anything useful at the moment
  if strings.ToLower(charset) == "iso-8859-1" {
    return r, nil
  }
  return nil, errors.New("Unsupported encoding: " + charset)
}

func UnmarshalStream(url string, reader io.Reader) (*Feed, error) {
  // Read the stream into memory (we'll need to parse it twice)
  var contentReader *bytes.Reader
  if buffer, err := ioutil.ReadAll(reader); err == nil {
    contentReader = bytes.NewReader(buffer)
  } else {
    return nil, err
  }

  genericFeed := GenericFeed{}

  decoder := xml.NewDecoder(contentReader)
  decoder.CharsetReader = charsetReader

  if err := decoder.Decode(&genericFeed); err != nil {
     return nil, err
  }

  var xmlFeed FeedMarshaler

  if genericFeed.XMLName.Space == "http://www.w3.org/1999/02/22-rdf-syntax-ns#" && genericFeed.XMLName.Local == "RDF" {
    xmlFeed = &rss1Feed { }
  } else if genericFeed.XMLName.Local == "rss" {
    xmlFeed = &rss2Feed { }
  } else if genericFeed.XMLName.Space == "http://www.w3.org/2005/Atom" && genericFeed.XMLName.Local == "feed" {
    xmlFeed = &atomFeed { }
  } else {
    return nil, errors.New("Unsupported type of feed (" +
      genericFeed.XMLName.Space + ":" + genericFeed.XMLName.Local + ")")
  }

  contentReader.Seek(0, 0)

  decoder = xml.NewDecoder(contentReader)
  decoder.CharsetReader = charsetReader

  if err := decoder.Decode(xmlFeed); err != nil {
    return nil, err
  }
  
  feed, err := xmlFeed.Marshal()
  feed.URL = url
  feed.Retrieved = time.Now().UTC()

  if err != nil {
    return nil, err
  }

  return &feed, nil
}

func parseTime(supportedFormats []string, timeSpec string) (time.Time, error) {
  if timeSpec != "" {
    for _, format := range supportedFormats {
      if parsedTime, err := time.Parse(format, timeSpec); err == nil {
        return parsedTime.UTC(), nil
      }
    }

    return time.Time {}, errors.New("Unrecognized time format: " + timeSpec)
  }

  return time.Time {}, nil
}

func substr(s string, pos int, length int) string {
  runes := []rune(s)
  l := pos + length
  if l > len(runes) {
    l = len(runes)
  }

  return string(runes[pos:l])
}
