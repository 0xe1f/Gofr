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
)

func newUserKey(c appengine.Context, userId UserID) *datastore.Key {
  return datastore.NewKey(c, "User", string(userId), 0, nil)
}

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

func unsubscribe(c appengine.Context, ancestorKey *datastore.Key) error {
  batchSize := 1000
  pending := 0
  articleKeys := make([]*datastore.Key, batchSize)

  q := datastore.NewQuery("Article").Ancestor(ancestorKey).KeysOnly()
  for t := q.Run(c); ; pending++ {
    articleKey, err := t.Next(nil)

    if err == datastore.Done || pending + 1 >= batchSize {
      // Delete the batch
      if pending > 0 {
        if err := datastore.DeleteMulti(c, articleKeys[:pending]); err != nil {
          return err
        }
      }

      pending = 0
      if err == datastore.Done {
        break
      }
    } else if err != nil {
      return err
    }

    articleKeys[pending] = articleKey
  }

  if ancestorKey.Kind() == "Folder" {
    // Remove subscriptions under the folder
    q = datastore.NewQuery("Subscription").Ancestor(ancestorKey).KeysOnly().Limit(1000)
    if subscriptionKeys, err := q.GetAll(c, nil); err == nil {
      if err := datastore.DeleteMulti(c, subscriptionKeys); err != nil {
        return err
      }
    }
  }

  if err := datastore.Delete(c, ancestorKey); err != nil {
    return err
  }

  return nil
}
