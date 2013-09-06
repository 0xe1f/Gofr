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

package sanitize

import (
  "fmt"
  "bytes"
  "strings"
  "unicode"
)

type content uint8

const (
  contentUnsafeText content = iota
  contentSafeText
  contentTag
  contentScript
  contentComment
)

func (content content)String() string {
  switch content {
  case contentUnsafeText:
    return "unsafeText"
  case contentSafeText:
    return "safeText"
  case contentTag:
    return "tag"
  case contentComment:
    return "comment"
  case contentScript:
    return "script"
  }

  return "unknown"
}

type Context struct {
  content content
  tagName string
}

func (context Context)String() string {
  return fmt.Sprintf("{ content: %s; tag: %s }", context.content, context.tagName)
}

var tagContent = map[string]content {
  "a":          contentSafeText,
  "address":    contentSafeText,
  "em":         contentSafeText,
  "strong":     contentSafeText,
  "b":          contentSafeText,
  "i":          contentSafeText,
  "big":        contentSafeText,
  "small":      contentSafeText,
  "sub":        contentSafeText,
  "sup":        contentSafeText,
  "cite":       contentSafeText,
  "code":       contentSafeText,
  "ol":         contentSafeText,
  "ul":         contentSafeText,
  "li":         contentSafeText,
  "dl":         contentSafeText,
  "lh":         contentSafeText,
  "dt":         contentSafeText,
  "dd":         contentSafeText,
  "p":          contentSafeText,
  "th":         contentSafeText,
  "td":         contentSafeText,
  "pre":        contentSafeText,
  "blockquote": contentSafeText,
  "h1":         contentSafeText,
  "h2":         contentSafeText,
  "h3":         contentSafeText,
  "h4":         contentSafeText,
  "h5":         contentSafeText,
  "h6":         contentSafeText,
  "div":        contentSafeText,
  "span":       contentSafeText,
  "ins":        contentSafeText,
  "del":        contentSafeText,
  "script":     contentScript,
}

func contentFromTag(tag string) content {
  return tagContent[tag]
}

func StripTags(html string) string {
  stack := Stack {}
  var buffer bytes.Buffer

  stack.Push(Context{ content: contentSafeText })

  i, bytes := 0, []byte(html)
  n := len(bytes) 
  writeStart, writeEnd := 0, 0

  for i < n {
    b := bytes[i]
    ctx := stack.Peek().(Context)

    if ctx.content == contentSafeText || ctx.content == contentUnsafeText{
      if b == '<' {
        if i + 3 < n && string(bytes[i:i + 4]) == "<!--" {
          // Comment
          stack.Push(Context{ content: contentComment })
          i += 3
        } else if i + 1 < n && unicode.IsLetter(rune(bytes[i + 1])) {
          // Opening tag
          j := i + 2
          for ; j < n; j++ {
            if !unicode.IsLetter(rune(bytes[j])) {
              break
            }
          }
          
          tagName := strings.ToLower(string(bytes[i + 1:j]))
          stack.Push(Context{ content: contentTag, tagName: tagName })

          i = j - 1
        } else if i + 2 < n && bytes[i + 1] == '/' && unicode.IsLetter(rune(bytes[i + 2])) {
          // Closing tag
          j := i + 3
          for ; j < n; j++ {
            if !unicode.IsLetter(rune(bytes[j])) {
              break
            }
          }

          tagName := strings.ToLower(string(bytes[i + 2:j]))

          // Consume everything up to the closing bracket
          for ; j < n; j++ {
            if bytes[j] == '>' {
              break
            }
          }

          popThisMany := 0
          if stack.Walk(func(value interface{}) bool {
            popThisMany++
            return value.(Context).tagName != tagName
          }) {
            stack.PopMany(popThisMany)
          }

          i = j
        } else if ctx.content == contentSafeText {
          if writeEnd < i {
            buffer.Write(bytes[writeStart:writeEnd])
            writeStart = i
          }

          writeEnd = i + 1
        }
      } else if ctx.content == contentSafeText {
        if writeEnd < i {
          buffer.Write(bytes[writeStart:writeEnd])
          writeStart = i
        }

        writeEnd = i + 1
      }
    } else if ctx.content == contentComment {
      if b == '>' && string(bytes[i - 2:i]) == "--" {
        stack.Pop()
      }
    } else if ctx.content == contentTag {
      if b == '"' || b == '\'' {
        // HTML attribute. Find the end
        j := i + 1
        for ; j < n; j++ {
          if bytes[j] == b {
            break
          }
        }
        i = j
      } else if b == '>' {
        // Closing bracket
        stack.Pop()

        if bytes[i - 1] != '/' {
          // Not a self-closing tag
          stack.Push(Context { content: contentFromTag(ctx.tagName), tagName: ctx.tagName })
        }
      }
    } else if ctx.content == contentScript {
      if i + 1 < n && b == '/' && bytes[i + 1] == '/' {
        // One-liner comment
        j := i + 2
        for ; j < n; j++ {
          if bytes[j] == '\n' {
            break
          }
        }

        i = j
      } else if i + 1 < n && b == '/' && bytes[i + 1] == '*' {
        // Multiline comment
        j := i + 2
        for ; j < n; j++ {
          if bytes[j] == '/' && bytes[j - 1] == '*' {
            break
          }
        }
        
        i = j
      } else if b == '"' || b == '\'' || b == '/' {
        // String or regex
        j := i + 1
        for ; j < n; j++ {
          if bytes[j] == '\\' {
            j++ // escaped
          } else if bytes[j] == b {
            break
          }
        }

        i = j
      } else if i + 2 < n && b == '<' && bytes[i + 1] == '/' && unicode.IsLetter(rune(bytes[i + 2])) {
        // Closing tag
        j := i + 3
        for ; j < n; j++ {
          if !unicode.IsLetter(rune(bytes[j])) {
            break
          }
        }

        tagName := strings.ToLower(string(bytes[i + 2:j]))

        // Consume everything up to the closing bracket
        for ; j < n; j++ {
          if bytes[j] == '>' {
            break
          }
        }

        if tagName == ctx.tagName {
          stack.Pop()
        }

        i = j
      }
    }

    i++
  }

  buffer.Write(bytes[writeStart:writeEnd])

  return buffer.String()
}
