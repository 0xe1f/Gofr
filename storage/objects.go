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
	"appengine/datastore"
	"encoding/json"
	"time"
)

const (
	likeCountShards = 20
)

type User struct {
	ID string
	EmailAddress string
	LastSubscriptionUpdate time.Time
}

type FeedMeta struct {
	Feed *datastore.Key
	InfoDigest []byte
	Fetched time.Time
	NextFetch time.Time
	UpdateCounter int64
	HourlyUpdateFrequency float32
}

type Feed struct {
	URL string
	Title string
	Description string `datastore:",noindex"`
	Link string
	Format string      `datastore:",noindex"`
	Topic string
	HubURL string
	FavIconURL string  `datastore:",noindex"`
	Updated time.Time
}

type FeedUsage struct {
	UpdateCount int64
	LastSubscriptionUpdate time.Time
	Feed *datastore.Key
}

type EntryMeta struct {
	Fetched time.Time
	Published time.Time
	InfoDigest []byte
	UpdateIndex int64
	Entry *datastore.Key
}

type Entry struct {
	Author string       `json:"author"`
	Title string        `json:"title"`
	Link string         `json:"link"`
	HasMedia bool       `json:"-"`
	Updated time.Time   `json:"-"`

	Media []EntryMedia  `json:"media"   datastore:"-"`
	Content string      `json:"content" datastore:",noindex"`
	Summary string      `json:"summary" datastore:",noindex"`
}

type EntryMedia struct {
	URL string           `json:"url"`
	Type string          `json:"-"`
	Entry *datastore.Key `json:"-"`
}

type likeCountShard struct {
	Entry *datastore.Key
	LikeCount int
}

type UserSubscriptions struct {
	Subscriptions []Subscription `json:"subscriptions"`
	Folders []Folder             `json:"folders"`
}

type UserID string

type FolderRef struct {
	UserID UserID
	FolderID string `json:"f"`
}

type SubscriptionRef struct {
	FolderRef
	SubscriptionID string  `json:"s"`
}

type ArticleExtras struct {
	LikeCount int `json:"likeCount"`
}

type ArticleScope SubscriptionRef

func SubscriptionRefFromJSON(userID UserID, refAsJSON string) (SubscriptionRef, error) {
	ref := SubscriptionRef{}
	if err := json.Unmarshal([]byte(refAsJSON), &ref); err != nil {
		return ref, err
	}

	ref.UserID = userID
	return ref, nil
}

func ArticleScopeFromJSON(userID UserID, scopeAsJSON string) (ArticleScope, error) {
	ref, err := SubscriptionRefFromJSON(userID, scopeAsJSON)
	return ArticleScope(ref), err
}

func ArticleFilterFromJSON(userID UserID, filterAsJSON string) (ArticleFilter, error) {
	filter := ArticleFilter{}
	if err := json.Unmarshal([]byte(filterAsJSON), &filter); err != nil {
		return filter, err
	}

	filter.UserID = userID
	return filter, nil
}

func (ref SubscriptionRef)IsSubscriptionExplicit() bool {
	return ref.SubscriptionID != ""
}

type ArticleFilter struct {
	ArticleScope
	Property string `json:"p"`
}

type ArticleRef struct {
	SubscriptionRef
	ArticleID string
}

type Subscription struct {
	ID string         `datastore:"-" json:"id"`
	Link string       `datastore:"-" json:"link"`
	FavIconURL string `datastore:"-" json:"favIconUrl"`
	Parent string     `datastore:"-" json:"parent,omitempty"`

	Updated time.Time    `json:"-"`
	Subscribed time.Time `json:"-"`
	Feed *datastore.Key  `json:"-"`
	MaxUpdateIndex int64 `json:"-"`

	Title string         `json:"title"`
	UnreadCount int      `json:"unread"`
}

type ArticlePage struct {
	Articles []Article `json:"articles"`
	Continue string    `json:"continue,omitempty"`
}

type Article struct {
	ID string             `datastore:"-" json:"id"`
	Source string         `datastore:"-" json:"source"`
	Details *Entry        `datastore:"-" json:"details"`

	UpdateIndex int64     `json:"-"`
	Fetched time.Time     `json:"time"`
	Published time.Time   `json:"published"`
	Entry *datastore.Key  `json:"-"`

	Properties []string   `json:"properties"`
}

type StorageInfo struct {
	Version int
}

type Folder struct {
	ID string    `datastore:"-" json:"id"`
	Title string `json:"title"`
}

func (article Article)IsUnread() bool {
	return article.HasProperty("unread")
}

func (article Article)IsLiked() bool {
	return article.HasProperty("like")
}

func (article Article)HasProperty(propName string) bool {
	for _, property := range article.Properties {
		if property == propName {
			return true
		}
	}

	return false
}

func (article *Article)SetProperty(propName string, value bool) {
	propMap := make(map[string]bool)
	for _, property := range article.Properties {
		propMap[property] = true
	}

	if value && !propMap[propName] {
		propMap[propName] = true
		if propName == "read" {
			delete(propMap, "unread")
		} else if propName == "unread" {
			delete(propMap, "read")
		}
	} else if !value && propMap[propName] {
		delete(propMap, propName)
		if propName == "read" {
			propMap["unread"] = true
		} else if propName == "unread" {
			propMap["read"] = true
		}
	}

	article.Properties = make([]string, len(propMap))
	i := 0

	for key, _ := range propMap {
		article.Properties[i] = key
		i++
	}
}

func (article *Article)ToggleProperty(propName string) {
	article.SetProperty(propName, !article.HasProperty(propName))
}
