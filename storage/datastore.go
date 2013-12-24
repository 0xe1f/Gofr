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
	"bytes"
	"errors"
	"fmt"
	"html"
	"math/rand"
	"rss"
	"time"
)

const (
	articlePageSize = 40
	defaultBatchSize = 400
)

func NewBatchWriter(c appengine.Context, op BatchOp) *BatchWriter {
	return NewBatchWriterWithSize(c, op, defaultBatchSize)
}

func IsFieldMismatch(err error) bool {
	_, ok := err.(*datastore.ErrFieldMismatch)
	return ok
}

func (ref FolderRef)IsZero() bool {
	return ref.UserID == "" && ref.FolderID == ""
}

func (userID UserID)key(c appengine.Context) (*datastore.Key, error) {
	if userID == "" {
		return nil, errors.New("UserID is empty")
	}

	return datastore.NewKey(c, "User", string(userID), 0, nil), nil
}

func (ref FolderRef)key(c appengine.Context) (*datastore.Key, error) {
	userKey, err := ref.UserID.key(c)
	if err != nil {
		return nil, err
	}

	if ref.FolderID != "" {
		if kind, id, err := unformatId(ref.FolderID); err != nil {
			return nil, err
		} else if kind == "folder" {
			return datastore.NewKey(c, "Folder", "", id, userKey), nil
		} else {
			return nil, errors.New("Expecting folder ID; found: " + kind)
		}
	}

	return userKey, nil
}

func (ref SubscriptionRef)key(c appengine.Context) (*datastore.Key, error) {
	ancestorKey, err := ref.FolderRef.key(c)
	if err != nil {
		return nil, err
	}

	if ref.SubscriptionID == "" {
		return nil, errors.New("SubscriptionRef is missing Subscription ID")
	}

	return datastore.NewKey(c, "Subscription", ref.SubscriptionID, 0, ancestorKey), nil
}

func (scope ArticleScope)key(c appengine.Context) (*datastore.Key, error) {
	ancestorKey, err := scope.FolderRef.key(c)
	if err != nil {
		return nil, err
	}

	if scope.SubscriptionID == "" {
		return ancestorKey, nil
	}

	return datastore.NewKey(c, "Subscription", scope.SubscriptionID, 0, ancestorKey), nil
}

func (ref ArticleRef)key(c appengine.Context) (*datastore.Key, error) {
	if ref.SubscriptionRef.SubscriptionID == "" {
		return nil, errors.New("Article reference is missing subscription ID")
	}

	subscriptionKey, err := ref.SubscriptionRef.key(c)
	if err != nil {
		return nil, err
	}

	return datastore.NewKey(c, "Article", ref.ArticleID, 0, subscriptionKey), nil
}

func (user User)key(c appengine.Context) (*datastore.Key, error) {
	if user.ID == "" {
		return nil, errors.New("User missing an ID")
	}

	return datastore.NewKey(c, "User", user.ID, 0, nil), nil
}

func NewArticlePage(c appengine.Context, filter ArticleFilter, start string) (*ArticlePage, error) {
	scopeKey, err := filter.key(c)
	if err != nil {
		return nil, err
	}

	q := datastore.NewQuery("Article").Ancestor(scopeKey).Order("-Fetched").Order("-Published")
	if filter.Property != "" {
		q = q.Filter("Properties = ", filter.Property)
	} else if filter.Tag != "" {
		q = q.Filter("Tags = ", filter.Tag)
	}

	if start != "" {
		if cursor, err := datastore.DecodeCursor(start); err == nil {
			q = q.Start(cursor)
		} else {
			return nil, err
		}
	}

	t := q.Run(c)

	articles := make([]Article, articlePageSize)
	entryKeys := make([]*datastore.Key, articlePageSize)

	var readCount int
	for readCount = 0; readCount < articlePageSize; readCount++ {
		article := &articles[readCount]

		if _, err := t.Next(article); err != nil && err == datastore.Done {
			break
		} else if IsFieldMismatch(err) {
			// Ignore - migration issue
		} else if err != nil {
			return nil, err
		}

		entryKey := article.Entry
		
		article.ID = entryKey.StringID()
		article.Source = entryKey.Parent().StringID()

		entryKeys[readCount] = entryKey
	}

	continueFrom := ""
	if readCount >= articlePageSize {
		if cursor, err := t.Cursor(); err == nil {
			continueFrom = cursor.String()
		}
	}

	articles = articles[:readCount]
	entryKeys = entryKeys[:readCount]

	entries := make([]Entry, readCount)
	if err := datastore.GetMulti(c, entryKeys, entries); err != nil {
		if multiError, ok := err.(appengine.MultiError); ok {
			for _, singleError := range multiError {
				if singleError != nil {
					// Safely ignore ErrFieldMismatch
					if !IsFieldMismatch(singleError) {
						return nil, err
					}
				}
			}
		} else {
			return nil, err
		}
	}

	for i, _ := range articles {
		if entries[i].HasMedia {
			if media, err := MediaForEntry(c, entryKeys[i]); err != nil {
				c.Warningf("Error loading media for entry: %s", err)
			} else {
				articles[i].Media = media
			}
		}
		articles[i].Details = &entries[i]
		if articles[i].Tags == nil {
			articles[i].Tags = make([]string, 0)
		}
	}

	page := ArticlePage {
		Articles: articles,
		Continue: continueFrom,
	}

	return &page, nil
}

func NewUserSubscriptions(c appengine.Context, userID UserID) (*UserSubscriptions, error) {
	var subscriptions []Subscription
	var subscriptionKeys []*datastore.Key

	userKey, err := userID.key(c)
	if err != nil {
		return nil, err
	}

	q := datastore.NewQuery("Subscription").Ancestor(userKey).Limit(defaultBatchSize)
	if subKeys, err := q.GetAll(c, &subscriptions); err != nil {
		return nil, err
	} else if subscriptions == nil {
		subscriptions = make([]Subscription, 0)
	} else {
		subscriptionKeys = subKeys
	}

	totalUnreadCount := 0
	feedKeys := make([]*datastore.Key, len(subscriptions))
	for i, subscription := range subscriptions {
		feedKeys[i] = subscription.Feed
		totalUnreadCount += subscription.UnreadCount
	}

	feeds := make([]Feed, len(subscriptions))

	if err := datastore.GetMulti(c, feedKeys, feeds); err != nil {
		if multiError, ok := err.(appengine.MultiError); ok {
			for _, singleError := range multiError {
				if singleError != nil {
					// Safely ignore ErrFieldMismatch
					if !IsFieldMismatch(singleError) {
						// It's not ErrFieldMismatch, 
						// but ignore the error anyway. We just
						// won't have a feed link
					}
				}
			}
		} else {
			return nil, err
		}
	}

	for i, _ := range subscriptions {
		subscriptionKey := subscriptionKeys[i]

		subscription := &subscriptions[i]
		subscription.ID = subscriptionKey.StringID()
		subscription.Link = feeds[i].Link
		subscription.FavIconURL = feeds[i].FavIconURL

		if subscriptionKey.Parent().Kind() == "Folder" {
			subscription.Parent = formatId("folder", subscriptionKey.Parent().IntID())
		}
	}

	// Get all folders
	var folders []Folder
	var folderKeys []*datastore.Key

	q = datastore.NewQuery("Folder").Ancestor(userKey).Limit(defaultBatchSize)
	if k, err := q.GetAll(c, &folders); err != nil {
		return nil, err
	} else if folders == nil {
		folders = make([]Folder, 0)
	} else {
		folderKeys = k
	}

	for i, _ := range folders {
		folder := &folders[i]
		folder.ID = formatId("folder", folderKeys[i].IntID())
	}

	// Get all tags
	var tags []Tag
	q = datastore.NewQuery("Tag").Ancestor(userKey).Limit(defaultBatchSize)
	if _, err := q.GetAll(c, &tags); err != nil {
		return nil, err
	} else if tags == nil {
		tags = make([]Tag, 0)
	}

	userSubscriptions := UserSubscriptions {
		Subscriptions: subscriptions,
		Folders: folders,
		Tags: tags,
	}

	return &userSubscriptions, nil
}

func IsFolderDuplicate(c appengine.Context, userID UserID, title string) (bool, error) {
	userKey, err := userID.key(c)
	if err != nil {
		return false, err
	}

	var folders []*Folder
	q := datastore.NewQuery("Folder").Ancestor(userKey).Filter("Title =", title).Limit(1)
	if _, err := q.GetAll(c, &folders); err == nil && len(folders) > 0 {
		return true, nil
	} else if err != nil {
		return false, err
	}

	return false, nil
}

func IsSubscriptionDuplicate(c appengine.Context, userID UserID, subscriptionURL string) (bool, error) {
	userKey, err := userID.key(c)
	if err != nil {
		return false, err
	}

	feedKey := datastore.NewKey(c, "Feed", subscriptionURL, 0, nil)
	q := datastore.NewQuery("Subscription").Ancestor(userKey).Filter("Feed =", feedKey).KeysOnly().Limit(1)

	if subKeys, err := q.GetAll(c, nil); err != nil {
		return false, err
	} else if len(subKeys) > 0 {
		return true, nil
	}

	return false, nil
}

func UserByID(c appengine.Context, userID UserID) (*User, error) {
	userKey := datastore.NewKey(c, "User", string(userID), 0, nil)
	user := User{}

	if err := datastore.Get(c, userKey, &user); err == nil {
		return &user, nil
	} else if err != datastore.ErrNoSuchEntity {
		return nil, err
	}

	return nil, nil
}

func (user User)Save(c appengine.Context) error {
	userKey, err := UserID(user.ID).key(c)
	if err != nil {
		return err
	}

	if _, err := datastore.Put(c, userKey, &user); err != nil {
		return err
	}

	return nil
}

func FolderByTitle(c appengine.Context, userID UserID, title string) (FolderRef, error) {
	userKey, err := userID.key(c)
	if err != nil {
		return FolderRef{}, err
	}
	
	q := datastore.NewQuery("Folder").Ancestor(userKey).Filter("Title =", title).KeysOnly().Limit(1)
	if folderKeys, err := q.GetAll(c, nil); err != nil {
		return FolderRef{}, err
	} else if len(folderKeys) > 0 {
		return newFolderRef(userID, folderKeys[0]), nil
	}

	return FolderRef{}, nil
}

func FolderExists(c appengine.Context, ref FolderRef) (bool, error) {
	if folderKey, err := ref.key(c); err != nil {
		return false, err
	} else {
		folder := new(Folder)
		if err := datastore.Get(c, folderKey, folder); err == nil {
			return true, nil
		} else if err != datastore.ErrNoSuchEntity {
			return false, err
		}
	}

	return false, nil
}

func TagExists(c appengine.Context, userID UserID, tagID string) (bool, error) {
	userKey, err := userID.key(c)
	if err != nil {
		return false, err
	}

	tagKey := datastore.NewKey(c, "Tag", tagID, 0, userKey)
	tag := new(Tag)
	if err := datastore.Get(c, tagKey, tag); err == nil || IsFieldMismatch(err) {
		return true, nil
	} else if err != datastore.ErrNoSuchEntity {
		return false, err
	}

	return false, nil
}

func SubscriptionExists(c appengine.Context, ref SubscriptionRef) (bool, error) {
	if subscriptionKey, err := ref.key(c); err != nil {
		return false, err
	} else {
		subscription := new(Subscription)
		if err := datastore.Get(c, subscriptionKey, subscription); err == nil {
			return true, nil
		} else if err != datastore.ErrNoSuchEntity {
			return false, err
		}
	}

	return false, nil
}

func CreateFolder(c appengine.Context, userID UserID, title string) (FolderRef, error) {
	userKey, err := userID.key(c)
	if err != nil {
		return FolderRef{}, err
	}
	
	folderKey := datastore.NewIncompleteKey(c, "Folder", userKey)
	folder := Folder {
		Title: title,
	}

	if completeKey, err := datastore.Put(c, folderKey, &folder); err != nil {
		return FolderRef{}, err
	} else {
		return newFolderRef(userID, completeKey), nil
	}
}

func RenameSubscription(c appengine.Context, ref SubscriptionRef, title string) error {
	subscriptionKey, err := ref.key(c)
	if err != nil {
		return err
	}

	subscription := new(Subscription)
	if err := datastore.Get(c, subscriptionKey, subscription); err != nil {
		return err
	}

	subscription.Title = title
	if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
		return err
	}

	return nil
}

func RenameFolder(c appengine.Context, ref FolderRef, title string) error {
	folderKey, err := ref.key(c)
	if err != nil {
		return err
	}

	folder := new(Folder)
	if err := datastore.Get(c, folderKey, folder); err != nil {
		return err
	}

	folder.Title = title
	if _, err := datastore.Put(c, folderKey, folder); err != nil {
		return err
	}

	return nil
}

func SetProperty(c appengine.Context, ref ArticleRef, propertyName string, propertyValue bool) ([]string, error) {
	articleKey, err := ref.key(c)
	if err != nil {
		return nil, err
	}

	article := new(Article)
	if err := datastore.Get(c, articleKey, article); err != nil && !IsFieldMismatch(err) {
		return nil, err
	}

	if propertyValue != article.HasProperty(propertyName) {
		wasUnread := article.IsUnread()
		wasLiked := article.IsLiked()
		unreadDelta := 0

		article.SetProperty(propertyName, propertyValue)

		// Update unread counts if necessary
		if wasUnread != article.IsUnread() {
			if wasUnread {
				unreadDelta = -1
			} else {
				unreadDelta = 1
			}
		}

		if _, err := datastore.Put(c, articleKey, article); err != nil {
			return nil, err
		}

		if wasLiked != article.IsLiked() {
			if wasLiked {
				article.updateLikeCount(c, -1)
			} else {
				article.updateLikeCount(c, 1)
			}
		}

		if unreadDelta != 0 {
			// Update unread counts - not critical
			subscriptionKey := articleKey.Parent()
			subscription := new(Subscription)

			if err := datastore.Get(c, subscriptionKey, subscription); err != nil {
				c.Warningf("Unread count update failed: subscription read error (%s)", err)
			} else if subscription.UnreadCount + unreadDelta >= 0 {
				subscription.UnreadCount += unreadDelta
				if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
					c.Warningf("Unread count update failed: subscription write error (%s)", err)
				}
			}
		}
	}

	return article.Properties, nil
}

func SetTags(c appengine.Context, ref ArticleRef, tags []string) ([]string, error) {
	articleKey, err := ref.key(c)
	if err != nil {
		return nil, err
	}

	article := new(Article)
	if err := datastore.Get(c, articleKey, article); err != nil && !IsFieldMismatch(err) {
		return nil, err
	}

	article.Tags = tags

	if _, err := datastore.Put(c, articleKey, article); err != nil {
		return nil, err
	}

	var userKey *datastore.Key
	if key, err := ref.UserID.key(c); err != nil {
		return nil, err
	} else {
		userKey = key
	}

	batchWriter := NewBatchWriter(c, BatchPut)
	for _, tagTitle := range tags {
		tagKey := datastore.NewKey(c, "Tag", tagTitle, 0, userKey)
		tag := Tag{}

		if err := datastore.Get(c, tagKey, &tag); err == nil || IsFieldMismatch(err) {
			// Already available
		} else if err == datastore.ErrNoSuchEntity {
			// Not yet available - add it
			tag.Title = tagTitle
			tag.Created = time.Now()

			if err := batchWriter.Enqueue(tagKey, &tag); err != nil {
				c.Errorf("Error queueing tag for batch write: %s", err)
				return nil, err
			}
		} else {
			// Some other error
			return nil, err
		}
	}

	if err := batchWriter.Flush(); err != nil {
		c.Errorf("Error flushing batch queue: %s", err)
		return nil, err
	}

	return article.Tags, nil
}

func MarkAllAsRead(c appengine.Context, scope ArticleScope) (int, error) {
	key, err := scope.key(c)
	if err != nil {
		return 0, err
	}

	batchWriter := NewBatchWriter(c, BatchPut)

	q := datastore.NewQuery("Article").Ancestor(key).Filter("Properties =", "unread")
	for t := q.Run(c); ; {
		article := new(Article)
		articleKey, err := t.Next(article)

		if err == datastore.Done {
			break
		} else if IsFieldMismatch(err) {
			// Ignore - migration issue
		} else if err != nil {
			c.Errorf("Error reading Article: %s", err)
			return 0, err
		}

		article.SetProperty("read", true)

		if err := batchWriter.Enqueue(articleKey, article); err != nil {
			c.Errorf("Error queueing article for batch write: %s", err)
			return 0, err
		}
	}

	if err := batchWriter.Flush(); err != nil {
		c.Errorf("Error flushing batch queue: %s", err)
		return 0, err
	}

	// Reset unread counters
	if key.Kind() != "Subscription" {
		var subscriptions []*Subscription
		q = datastore.NewQuery("Subscription").Ancestor(key).Limit(defaultBatchSize)
		
		if subscriptionKeys, err := q.GetAll(c, &subscriptions); err != nil {
			return 0, err
		} else {
			for _, subscription := range subscriptions {
				subscription.UnreadCount = 0
			}

			if _, err := datastore.PutMulti(c, subscriptionKeys, subscriptions); err != nil {
				return 0, err
			}
		}
	} else {
		subscription := new(Subscription)
		if err := datastore.Get(c, key, subscription); err != nil {
			return 0, err
		}

		subscription.UnreadCount = 0
		if _, err := datastore.Put(c, key, subscription); err != nil {
			return 0, err
		}
	}

	return batchWriter.Written(), nil
}

func MoveSubscription(c appengine.Context, subRef SubscriptionRef, destRef FolderRef) error {
	currentSubscriptionKey, err := subRef.key(c)
	if err != nil {
		return err
	}

	newSubRef := SubscriptionRef {
		FolderRef: destRef,
		SubscriptionID: subRef.SubscriptionID,
	}

	newSubscriptionKey, err := newSubRef.key(c)
	if err != nil {
		return err
	}

	subscription := new(Subscription)
	if err := datastore.Get(c, currentSubscriptionKey, subscription); err != nil {
		c.Errorf("Error reading subscription: %s", err)
		return err
	}

	if _, err := datastore.Put(c, newSubscriptionKey, subscription); err != nil {
		c.Errorf("Error writing subscription: %s", err)
		return err
	}

	if err := datastore.Delete(c, currentSubscriptionKey); err != nil {
		c.Errorf("Error deleting subscription: %s", err)
		return err
	}

	return nil
}

func MoveArticles(c appengine.Context, subRef SubscriptionRef, destRef FolderRef) error {
	currentSubscriptionKey, err := subRef.key(c)
	if err != nil {
		return err
	}

	newSubRef := SubscriptionRef {
		FolderRef: destRef,
		SubscriptionID: subRef.SubscriptionID,
	}

	newSubscriptionKey, err := newSubRef.key(c)
	if err != nil {
		return err
	}

	batchWriter := NewBatchWriter(c, BatchPut)
	batchDeleter := NewBatchWriter(c, BatchDelete)

	q := datastore.NewQuery("Article").Ancestor(currentSubscriptionKey)
	for t := q.Run(c); ; {
		article := new(Article)
		currentArticleKey, err := t.Next(article)

		if err == datastore.Done {
			break
		} else if IsFieldMismatch(err) {
			// Safely ignore - migration issue
		} else if err != nil {
			c.Errorf("Error reading Article: %s", err)
			return err
		}

		newArticleKey := datastore.NewKey(c, "Article", currentArticleKey.StringID(), 0, newSubscriptionKey)
		if err := batchWriter.Enqueue(newArticleKey, article); err != nil {
			c.Errorf("Error queueing article for batch write: %s", err)
			return err
		}
		if err := batchDeleter.EnqueueKey(currentArticleKey); err != nil {
			c.Errorf("Error queueing article for batch delete: %s", err)
			return err
		}
	}

	if err := batchWriter.Flush(); err != nil {
		c.Errorf("Error flushing batch write queue: %s", err)
		return err
	}

	if err := batchDeleter.Flush(); err != nil {
		c.Errorf("Error flushing batch delete queue: %s", err)
		return err
	}

	return nil
}

func DeleteFolder(c appengine.Context, ref FolderRef) error {
	folderKey, err := ref.key(c)
	if err != nil {
		return err
	}

	// Get a list of relevant subscriptions
	q := datastore.NewQuery("Subscription").Ancestor(folderKey).KeysOnly().Limit(defaultBatchSize)
	subscriptionKeys, err := q.GetAll(c, nil)

	if err != nil {
		return err
	}

	// Delete folder & subscriptions
	if err := datastore.Delete(c, folderKey); err != nil {
		c.Errorf("Error deleting folder: %s", err)
		return err
	}

	if subscriptionKeys == nil {
		// No subscriptions; nothing more to do
		return nil
	}

	if err := datastore.DeleteMulti(c, subscriptionKeys); err != nil {
		c.Errorf("Error deleting subscriptions: %s", err)
		return err
	}

	return nil
}

func DeleteTag(c appengine.Context, userID UserID, tagID string) error {
	userKey, err := userID.key(c)
	if err != nil {
		return err
	}

	tagKey := datastore.NewKey(c, "Tag", tagID, 0, userKey)
	if err := datastore.Delete(c, tagKey); err != nil {
		c.Errorf("Error deleting tag: %s", err)
		return err
	}

	return nil
}

func FeedByURL(c appengine.Context, url string) (*Feed, error) {
	feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
	feed := new(Feed)

	if err := datastore.Get(c, feedKey, feed); err == nil || IsFieldMismatch(err) {
		return feed, nil
	} else if err != datastore.ErrNoSuchEntity {
		return nil, err
	}

	return nil, nil
}

func IsFeedAvailable(c appengine.Context, url string) (bool, error) {
	feedKey := datastore.NewKey(c, "Feed", url, 0, nil)
	feed := new(Feed)

	if err := datastore.Get(c, feedKey, feed); err == nil {
		return true, nil
	} else if IsFieldMismatch(err) {
		// Some fields don't match - migration issue
		return true, nil
	} else if err != datastore.ErrNoSuchEntity {
		return false, err
	}

	return false, nil
}

func WebToFeedURL(c appengine.Context, url string, title *string) (string, error) {
	q := datastore.NewQuery("Feed").Filter("Link =", url)
	t := q.Run(c)
	
	feed := new(Feed)
	if _, err := t.Next(feed); err == nil || IsFieldMismatch(err) {
		if title != nil {
			*title = feed.Title
		}
		return feed.URL, nil
	} else if err == datastore.Done {
		// No match
	} else if err != nil {
		return "", err
	}

	return "", nil
}

func Subscribe(c appengine.Context, ref FolderRef, url string, title string) (SubscriptionRef, error) {
	folderKey, err := ref.key(c)
	if err != nil {
		return SubscriptionRef{}, err
	}

	subscription := new(Subscription)
	subscriptionKey := datastore.NewKey(c, "Subscription", url, 0, folderKey)

	if err := datastore.Get(c, subscriptionKey, subscription); err == nil {
		return SubscriptionRef{
			FolderRef: ref,
			SubscriptionID: url,
		}, nil // Already subscribed
	} else if err == datastore.ErrNoSuchEntity {
		subscription.Updated = time.Time {}
		subscription.Subscribed = time.Now()
		subscription.Title = title
		subscription.UnreadCount = 0
		subscription.MaxUpdateIndex = -1
		subscription.Feed = datastore.NewKey(c, "Feed", url, 0, nil)
	} else {
		return SubscriptionRef{}, err
	}

	if _, err := datastore.Put(c, subscriptionKey, subscription); err != nil {
		return SubscriptionRef{}, err
	}

	if err := updateSubscriberCount(c, url, 1); err != nil {
		c.Warningf("Error incrementing subscriber count: %s", err)
	}

	return SubscriptionRef{
		FolderRef: ref,
		SubscriptionID: url,
	}, nil
}

func SubscriptionsAsOPML(c appengine.Context, userID UserID) (*rss.OPML, error) {
	userKey, err := userID.key(c)
	if err != nil {
		return nil, err
	}

	opml := rss.NewOPML()
	folderMap := map[string]*rss.Outline{}
	q := datastore.NewQuery("Folder").Ancestor(userKey).Limit(defaultBatchSize)

	var folders []Folder
	if folderKeys, err := q.GetAll(c, &folders); err != nil {
		return nil, err
	} else if folders != nil {
		for i, folder := range folders {
			opmlFolder := rss.NewFolder(folder.Title)
			folderMap[folderKeys[i].String()] = opmlFolder
			opml.Add(opmlFolder)
		}
	}

	q = datastore.NewQuery("Subscription").Ancestor(userKey).Limit(defaultBatchSize)

	var subscriptions []Subscription
	if subscriptionKeys, err := q.GetAll(c, &subscriptions); err != nil {
		return nil, err
	} else {
		feedKeys := make([]*datastore.Key, len(subscriptions))
		for i, subscription := range subscriptions {
			feedKeys[i] = subscription.Feed
		}

		var multiError appengine.MultiError
		feeds := make([]Feed, len(subscriptions))

		if err := datastore.GetMulti(c, feedKeys, feeds); err != nil {
			if me, ok := err.(appengine.MultiError); ok {
				multiError = me
			} else {
				return nil, err
			}
		}

		for i, subscription := range subscriptions {
			subscriptionKey := subscriptionKeys[i]
			parentKey := subscriptionKey.Parent()

			opmlSub := rss.NewSubscription(subscription.Title, subscriptionKey.StringID(), "")
			if parentKey.Kind() != "Folder" {
				opml.Add(opmlSub)
			} else {
				if folder := folderMap[parentKey.String()]; folder != nil {
					folder.Add(opmlSub)
				} else {
					opml.Add(opmlSub) // Orphaned folder
				}
			}

			if multiError == nil || multiError[i] == nil {
				opmlSub.WebURL = feeds[i].Link
			}
		}
	}

	return &opml, nil
}

func Unsubscribe(c appengine.Context, ref SubscriptionRef) error {
	subscriptionKey, err := ref.key(c)
	if err != nil {
		return err
	}

	if err := datastore.Delete(c, subscriptionKey); err != nil {
		return err
	}

	if err := updateSubscriberCount(c, ref.SubscriptionID, -1); err != nil {
		c.Warningf("Error decrementing subscriber count: %s", err)
	}

	return nil
}

func DeleteArticlesWithinScope(c appengine.Context, scope ArticleScope) error {
	ancestorKey, err := scope.key(c)
	if err != nil {
		return err
	}

	batchWriter := NewBatchWriter(c, BatchDelete)

	q := datastore.NewQuery("Article").Ancestor(ancestorKey).KeysOnly()
	for t := q.Run(c); ; {
		articleKey, err := t.Next(nil)

		if err == datastore.Done {
			break
		} else if err != nil {
			return err
		}

		if err := batchWriter.EnqueueKey(articleKey); err != nil {
			c.Errorf("Error queueing article for batch delete: %s", err)
			return err
		}
	}

	if err := batchWriter.Flush(); err != nil {
		c.Errorf("Error flushing batch queue: %s", err)
		return err
	}

	return nil
}

func RemoveTag(c appengine.Context, userID UserID, tag string) error {
	userKey, err := userID.key(c)
	if err != nil {
		return err
	}

	batchWriter := NewBatchWriter(c, BatchPut)

	q := datastore.NewQuery("Article").Ancestor(userKey).Filter("Tags = ", tag)
	for t := q.Run(c); ; {
		article := new(Article)
		articleKey, err := t.Next(article)

		if err == datastore.Done {
			break
		} else if IsFieldMismatch(err) {
			// Not a proper error
		} else if err != nil {
			return err
		}

		// Unset the tag
		article.SetTag(tag, false)

		// Queue for write
		if err := batchWriter.Enqueue(articleKey, article); err != nil {
			c.Errorf("Error queueing article for batch untag: %s", err)
			return err
		}
	}

	if err := batchWriter.Flush(); err != nil {
		c.Errorf("Error flushing batch queue: %s", err)
		return err
	}

	return nil
}

func UpdateFeed(c appengine.Context, parsedFeed *rss.Feed, favIconURL string, fetched time.Time) error {
	var updateCounter int64
	var lastFetched time.Time

	feedDigest := parsedFeed.Digest()
	feedMeta := new(FeedMeta)
	feedMetaKey := datastore.NewKey(c, "FeedMeta", parsedFeed.URL, 0, nil)
	feedKey := datastore.NewKey(c, "Feed", parsedFeed.URL, 0, nil)
	updateInfo := false

	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		if err := datastore.Get(c, feedMetaKey, feedMeta); err == datastore.ErrNoSuchEntity {
			// New; set defaults
			feedMeta.Feed = feedKey
			feedMeta.UpdateCounter = 0
			feedMeta.InfoDigest = feedDigest
			updateInfo = true
		} else if err == nil || IsFieldMismatch(err) {
			// If the feed information has changed, update it
			if !bytes.Equal(feedMeta.InfoDigest, feedDigest) {
				feedMeta.InfoDigest = feedDigest
				updateInfo = true
			}
		} else {
			// Some other error
			return err
		}

		durationBetweenUpdates := parsedFeed.DurationBetweenUpdates()

		lastFetched = feedMeta.Fetched

		feedMeta.Fetched = fetched
		feedMeta.NextFetch = fetched.Add(durationBetweenUpdates)
		feedMeta.HourlyUpdateFrequency = float32(durationBetweenUpdates.Hours())
		feedMeta.UpdateCounter += int64(len(parsedFeed.Entries))

		updateCounter = feedMeta.UpdateCounter

		if updatedKey, err := datastore.Put(c, feedMetaKey, feedMeta); err != nil {
			return err
		} else {
			feedMetaKey = updatedKey
		}

		return nil
	}, nil)

	if err != nil {
		c.Errorf("Error incrementing entry counter: %s", err)
		return err
	}

	// Consolidate subscriber count from shards
	if subscriberCount, err := consolidatedSubscriberCount(c, feedKey); err != nil {
		c.Warningf("Error reading subscriber count: %s", err)
	} else {
		feedSubKey := datastore.NewKey(c, "FeedSubscriber", parsedFeed.URL, 0, nil)
		feedSub := new(FeedSubscriber)
		updateCounts := false

		if err := datastore.Get(c, feedSubKey, feedSub); err == datastore.ErrNoSuchEntity {
			feedSub.Feed = feedKey
			updateCounts = true
		} else if err == nil || IsFieldMismatch(err) {
			// No error
			updateCounts = (feedSub.Count != subscriberCount)
		} else {
			// Some other error
			c.Warningf("Error reading FeedSubscriber entity: %s", err)
		}

		// If the count has changed, update it
		if updateCounts {
			feedSub.Count = subscriberCount
			if _, err := datastore.Put(c, feedSubKey, feedSub); err != nil {
				c.Warningf("Error writing FeedSubscriber entity: %s", err)
			}
		}
	}

	// Update information (Feed entity)
	if updateInfo {
		feed := new(Feed)
		if err := datastore.Get(c, feedKey, feed); err == datastore.ErrNoSuchEntity {
			feed.URL = parsedFeed.URL
		} else if err != nil {
			return err
		}

		if favIconURL != "" {
			// FavIcon URL will not be passed when updating
			feed.FavIconURL = favIconURL
		}

		feed.Title = parsedFeed.Title
		feed.Description = parsedFeed.Description
		feed.Updated = parsedFeed.Updated
		feed.Link = parsedFeed.WWWURL
		feed.Format = parsedFeed.Format
		feed.HubURL = parsedFeed.HubURL
		feed.Topic = parsedFeed.Topic

		if _, err := datastore.Put(c, feedKey, feed); err != nil {
			return err
		}
	}

	batchSize := defaultBatchSize
	elements := len(parsedFeed.Entries)

	entryKeys := make([]*datastore.Key, batchSize)
	entries := make([]*Entry, batchSize)

	entryMetaKeys := make([]*datastore.Key, batchSize)
	entryMetas := make([]*EntryMeta, batchSize)

	pending := 0
	written := 0

	started := time.Now()
	nuovo, unchanged, changed := 0, 0, 0

	for i := 0; ; i++ {
		if i >= elements || pending + 1 >= batchSize {
			if pending > 0 {
				if _, err := datastore.PutMulti(c, entryKeys[:pending], entries[:pending]); err != nil {
					if multiError, ok := err.(appengine.MultiError); ok {
						for i, err := range multiError {
							if err != nil {
								c.Errorf("entry[%d]: key [%s] failed: %s", i, 
									entryKeys[i].StringID(), err)
							}
						}
					}
					// FIXME: don't stop the entire write simply because a few entries failed
					return err
				}
				if _, err := datastore.PutMulti(c, entryMetaKeys[:pending], entryMetas[:pending]); err != nil {
					if multiError, ok := err.(appengine.MultiError); ok {
						for i, err := range multiError {
							if err != nil {
								c.Errorf("entryMeta[%d]: [%s] failed: %s", i, 
									entryMetaKeys[i].StringID(), err)
							}
						}
					}
					// FIXME: don't stop the entire write simply because a few entries failed
					return err
				}
			}

			written += pending
			pending = 0

			if i >= elements {
				break
			}
		}

		parsedEntry := parsedFeed.Entries[i]
		entryGUID := parsedEntry.UniqueID()
		if entryGUID == "" {
			c.Warningf("Missing GUID for an entry titled '%s'", parsedEntry.Title)
			continue
		}

		entryMetaKey := datastore.NewKey(c, "EntryMeta", entryGUID, 0, feedMeta.Feed)
		entryKey := datastore.NewKey(c, "Entry", entryGUID, 0, feedMeta.Feed)
		entryDigest := parsedEntry.Digest()
		var entryMeta EntryMeta

		if err := datastore.Get(c, entryMetaKey, &entryMeta); err == datastore.ErrNoSuchEntity {
			// New; set defaults
			entryMeta.Entry = entryKey
			entryMeta.InfoDigest = entryDigest
			nuovo++
		} else if err == nil || IsFieldMismatch(err) {
			if !bytes.Equal(entryMeta.InfoDigest, entryDigest) {
				entryMeta.InfoDigest = entryDigest
				changed++
			} else {
				// No updates - skip
				unchanged++
				continue
			}
		} else {
			// Some other error
			c.Warningf("Error getting entry meta (GUID '%s'): %s", entryGUID, err)
			continue
		}

		entryMeta.Published = parsedEntry.Published
		entryMeta.Fetched = fetched
		entryMeta.UpdateIndex = updateCounter

		// At this point, metadata tells us the record needs updating, so we 
		// just overwrite everything in the entry

		entry := Entry {
			Author: html.UnescapeString(parsedEntry.Author),
			Title: html.UnescapeString(parsedEntry.Title),
			Link: parsedEntry.WWWURL,
			Summary: parsedEntry.Summary(),
			Content: parsedEntry.Content,
			Updated: parsedEntry.Updated,
		}

		if len(parsedEntry.Media) > 0 {
			if err := UpdateMedia(c, entryKey, parsedEntry); err != nil {
				c.Warningf("Error writing media for entry: %s")
			} else {
				entry.HasMedia = true
			}
		}

		entryMetas[pending] = &entryMeta
		entryMetaKeys[pending] = entryMetaKey
		entries[pending] = &entry
		entryKeys[pending] = entryKey

		updateCounter++
		pending++
	}

	c.Debugf("Completed %s: %d,%d,%d (n,c,u) (took %s, last fetch: %s ago)", 
		parsedFeed.URL, nuovo, changed, unchanged, time.Since(started), time.Since(lastFetched))

	return nil
}

func MediaForEntry(c appengine.Context, entryKey *datastore.Key) ([]*EntryMedia, error) {
	mediaList := make([]*EntryMedia, 0, 40)
	q := datastore.NewQuery("EntryMedia").Filter("Entry =", entryKey)
	for t := q.Run(c); ; {
		entryMedia := new(EntryMedia)
		_, err := t.Next(entryMedia)

		if err == datastore.Done {
			break
		} else if IsFieldMismatch(err) {
			// Ignore - possibly a missing field
		} else if err != nil {
			return []*EntryMedia{}, err
		}

		mediaList = append(mediaList, entryMedia)
	}

	return mediaList, nil
}

func UpdateMedia(c appengine.Context, entryKey *datastore.Key, entry *rss.Entry) error {
	// Find and remove any existing media
	q := datastore.NewQuery("EntryMedia").Filter("Entry =", entryKey).KeysOnly().Limit(40)
	if entryMediaKeys, err := q.GetAll(c, nil); err != nil {
		return err
	} else if len(entryMediaKeys) > 0 {
		if datastore.DeleteMulti(c, entryMediaKeys); err != nil {
			return err
		}
	}

	batchWriter := NewBatchWriter(c, BatchPut)

	// Add media
	for _, media := range entry.Media {
		entryMediaKey := datastore.NewIncompleteKey(c, "EntryMedia", nil)
		entryMedia := EntryMedia {
			URL: media.URL,
			Type: media.Type,
			Entry: entryKey,
		}

		if err := batchWriter.Enqueue(entryMediaKey, &entryMedia); err != nil {
			c.Errorf("Error queueing media for batch write: %s", err)
			return err
		}
	}

	if err := batchWriter.Flush(); err != nil {
		c.Errorf("Error flushing batch queue: %s", err)
		return err
	}

	return nil
}

func UpdateSubscription(c appengine.Context, url string, ref SubscriptionRef) (int, error) {
	subscriptionKey, err := ref.key(c)
	if err != nil {
		return 0, err
	}

	subscription := Subscription{}
	if err := datastore.Get(c, subscriptionKey, &subscription); err != nil {
		c.Errorf("Error getting subscription: %s", err)
		return 0, err
	}

	return updateSubscriptionByKey(c, subscriptionKey, subscription)
}

func UpdateAllSubscriptions(c appengine.Context, userID UserID) error {
	userKey, err := userID.key(c)
	if err != nil {
		return err
	}
	
	var subscriptions []Subscription

	q := datastore.NewQuery("Subscription").Ancestor(userKey).Limit(defaultBatchSize)
	subscriptionKeys, err := q.GetAll(c, &subscriptions)
	if err != nil {
		return err
	}

	started := time.Now()
	doneChannel := make(chan Subscription)
	subscriptionCount := len(subscriptions)

	for i := 0; i < subscriptionCount; i++ {
		go updateSubscriptionAsync(c, subscriptionKeys[i], subscriptions[i], doneChannel)
	}

	for i := 0; i < subscriptionCount; i++ {
		<-doneChannel;
	}

	c.Infof("%d subscriptions completed in %s", subscriptionCount, time.Since(started))

	return nil
}

func AreNewEntriesAvailable(c appengine.Context, subscriptions []Subscription) (bool, error) {
	for _, subscription := range subscriptions {
		q := datastore.NewQuery("EntryMeta").Ancestor(subscription.Feed).Filter("UpdateIndex >", subscription.MaxUpdateIndex).KeysOnly().Limit(1)
		if entryMetaKeys, err := q.GetAll(c, nil); err != nil {
			return false, err
		} else if len(entryMetaKeys) > 0 {
			return true, nil
		}
	}

	return false, nil
}

func UpdateUnreadCounts(c appengine.Context, ch chan<- Subscription, subscriptionKey *datastore.Key, subscription Subscription) {
	originalSubscriptionCount := subscription.UnreadCount
	q := datastore.NewQuery("Article").Ancestor(subscriptionKey).Filter("Properties =", "unread")

	if count, err := q.Count(c); err != nil {
		c.Errorf("Error getting unread count: %s", err)
		goto done
	} else if count != originalSubscriptionCount {
		subscription.UnreadCount = count
		if _, err := datastore.Put(c, subscriptionKey, &subscription); err != nil {
			c.Errorf("Error writing unread count: %s", err)
			goto done
		}
	}

	if originalSubscriptionCount != subscription.UnreadCount {
		c.Infof("Subscription count corrected to %d (was: %d)", 
			subscription.UnreadCount, originalSubscriptionCount)
	}

done:
	if ch != nil {
		ch<- subscription
	}
}

func (article Article) LikeCount(c appengine.Context) (int, error) {
	count := 0
	q := datastore.NewQuery("LikeCountShard").Filter("Entry =", article.Entry)
	for t := q.Run(c); ; {
		var shard likeCountShard
		if _, err := t.Next(&shard); err == datastore.Done {
			break
		} else if err != nil {
			return count, err
		}
		count += shard.LikeCount
	}

	return count, nil
}

func (article Article) updateLikeCount(c appengine.Context, delta int) error {
	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		shardName := fmt.Sprintf("%s#%d", 
			article.Entry.StringID(), rand.Intn(likeCountShards))
		key := datastore.NewKey(c, "LikeCountShard", shardName, 0, nil)

		var shard likeCountShard
		if err := datastore.Get(c, key, &shard); err == datastore.ErrNoSuchEntity {
			shard.Entry = article.Entry
		} else if err != nil {
			return err
		}

		shard.LikeCount += delta
		_, err := datastore.Put(c, key, &shard)

		return err
	}, nil)

	if err != nil {
		return err
	}

	return nil
}

func consolidatedSubscriberCount(c appengine.Context, feedKey *datastore.Key) (int, error) {
	count := 0
	q := datastore.NewQuery("SubscriberCountShard").Filter("Feed =", feedKey)
	for t := q.Run(c); ; {
		var shard subscriberCountShard
		if _, err := t.Next(&shard); err == datastore.Done {
			break
		} else if err != nil {
			return count, err
		}
		count += shard.SubscriberCount
	}

	return count, nil
}

func updateSubscriberCount(c appengine.Context, feedURL string, delta int) error {
	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		shardName := fmt.Sprintf("%s#%d", 
			feedURL, rand.Intn(subscriberCountShards))
		key := datastore.NewKey(c, "SubscriberCountShard", shardName, 0, nil)

		var shard subscriberCountShard
		if err := datastore.Get(c, key, &shard); err == datastore.ErrNoSuchEntity {
			shard.Feed = datastore.NewKey(c, "Feed", feedURL, 0, nil)
		} else if err != nil {
			return err
		}

		shard.SubscriberCount += delta
		_, err := datastore.Put(c, key, &shard)

		return err
	}, nil)

	if err != nil {
		return err
	}

	return nil
}

func LoadArticleExtras(c appengine.Context, ref ArticleRef) (ArticleExtras, error) {
	articleKey, err := ref.key(c)
	if err != nil {
		return ArticleExtras{}, err
	}

	article := new(Article)
	if err := datastore.Get(c, articleKey, article); err != nil {
		return ArticleExtras{}, err
	}

	if likeCount, err := article.LikeCount(c); err != nil {
		return ArticleExtras{}, err
	} else {
		return ArticleExtras {
			LikeCount: likeCount,
		}, nil
	}
}
