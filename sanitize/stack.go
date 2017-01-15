/*****************************************************************************
 **
 ** Gofr
 ** https://github.com/pokebyte/Gofr
 ** Copyright (C) 2013-2017 Akop Karapetyan
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

package sanitize

import (
	"fmt"
	"bytes"
)

// Return 'false' to stop walking stack
type Walker func(value interface{}) bool

type stackItem struct {
	Value interface{}
	Previous *stackItem
}

type Stack struct {
	top *stackItem
}

func (stack *Stack)Push(value interface{}) interface{} {
	item := new(stackItem)

	item.Value = value
	item.Previous = stack.top

	stack.top = item

	return value
}

func (stack *Stack)Pop() interface{} {
	item := stack.top

	if item == nil {
		return nil
	}

	stack.top = item.Previous
	item.Previous = nil

	return item.Value
}

func (stack *Stack)PopMany(count int) interface{} {
	var value interface{}
	for i := 0; i < count; i++ {
		value = stack.Pop()
		if value == nil {
			break
		}
	}

	return value
}

func (stack *Stack)Peek() interface{} {
	if item := stack.top; item == nil {
		return nil
	} else {
		return item.Value
	}
}

// Returns 'true' if walk was stopped prematurely
func (stack *Stack)Walk(walker Walker) bool {
	for item := stack.top; item != nil; item = item.Previous {
		if !walker(item.Value) {
			return true
		}
	}

	return false
}

func (stack *Stack)String() string {
	buffer := bytes.Buffer{}
	for item := stack.top; item != nil; item = item.Previous {
		buffer.WriteString(item.Value.(fmt.Stringer).String())
		if item.Previous != nil {
			buffer.WriteString(" <- ")
		}
	}

	return buffer.String()
}
