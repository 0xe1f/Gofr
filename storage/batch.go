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
)

type BatchOp int

const (
  BatchPut BatchOp = iota
  BatchDelete
)

// NOTE: not safe across goroutines

type BatchWriter struct {
  c appengine.Context
  objects []interface{}
  keys []*datastore.Key
  size int
  pending int
  written int
  op BatchOp
}

func NewBatchWriter(c appengine.Context, op BatchOp) *BatchWriter {
  return NewBatchWriterWithSize(c, op, 400)
}

func NewBatchWriterWithSize(c appengine.Context, op BatchOp, size int) *BatchWriter {
  writer := BatchWriter {
    c: c,
    keys: make([]*datastore.Key, size),
    size: size,
    op: op,
  }

  if writer.supportsObjects() {
    writer.objects = make([]interface{}, size)
  }

  return &writer
}

func (writer BatchWriter)supportsObjects() bool {
  return writer.op != BatchDelete
}

func (writer *BatchWriter)EnqueueKey(key *datastore.Key) error {
  return writer.Enqueue(key, nil)
}

func (writer *BatchWriter)Enqueue(key *datastore.Key, object interface{}) error {
  if writer.pending + 1 >= writer.size {
    if err := writer.Flush(); err != nil {
      return err
    }
  }

  writer.keys[writer.pending] = key
  if writer.supportsObjects() {
    writer.objects[writer.pending] = object
  }

  writer.pending++

  return nil
}

func (writer *BatchWriter)Flush() error {
  if writer.pending > 0 {
    if writer.op == BatchPut {
      if _, err := datastore.PutMulti(writer.c, writer.keys[:writer.pending], writer.objects[:writer.pending]); err != nil {
        return err
      }
    } else if writer.op == BatchDelete {
      if err := datastore.DeleteMulti(writer.c, writer.keys[:writer.pending]); err != nil {
        return err
      }
    }

    writer.written += writer.pending
    writer.pending = 0
  }

  return nil
}

func (writer *BatchWriter)Close() error {
  return writer.Flush()
}

func (writer BatchWriter)Written() int {
  return writer.written
}
