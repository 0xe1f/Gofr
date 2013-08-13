/*****************************************************************************
 **
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
 
$().ready(function()
{
  var showToast = function(message, isError)
  {
    $('#toast span').text(message);
    $('#toast').attr('class', isError ? 'error' : 'info');

    if ($('#toast').is(':hidden'))
    {
      $('#toast')
        .fadeIn()
        .delay(8000)
        .fadeOut('slow'); 
    }
  };

  var _l = function(str, args)
  {
    // FIXME
    if (args)
      return vsprintf(str, args);

    return str;
  };

  var getPublishedDate = function(dateAsString)
  {
    var now = new Date();
    var date = new Date(dateAsString);
    
    var sameDay = now.getDate() == date.getDate() 
      && now.getMonth() == date.getMonth() 
      && now.getFullYear() == date.getFullYear();

    if (sameDay)
      return date.toLocaleTimeString();
    else
      return date.toLocaleDateString();
  };

  var getSubscription = function(subscriptionId)
  {
    var subscriptionMap = $('#feeds').data('sub-map');
    if (subscriptionMap != null)
      return subscriptionMap[subscriptionId];

    return null;
  };

  $(document).ajaxError(function(event, jqxhr, settings, exception) 
  {
    var errorMessage;

    try 
    {
      var errorJson = $.parseJSON(jqxhr.responseText)
      errorMessage = errorJson.errorMessage;
    }
    catch (exception)
    {
      errorMessage = _l("An unexpected error has occurred. Please try again later.");
    }

    showToast(errorMessage, true);
  });

  var loadSubscriptions = function()
  {
    $.getJSON('subscriptions', 
    {
    },
    function(subscriptions)
    {
      $('#feeds').empty();

      var subscriptionMap = new Object();

      $.each(subscriptions, function()
      {
        subscriptionMap[this.id] = this;

        var subDom = $('<li />', { 'class' : 'feed' })
          .data('sub-id', this.id)
          .append($('<div />', { 'class' : 'feed-item' })
            .append($('<span />', { 'class' : 'chevron' })
              .click(function(e)
              {
                // FIXME: show the menu
                e.stopPropagation();
              }))
            .append($('<div />', { 'class' : 'feed-icon' }))
            .append($('<span />', { 'class' : 'feed-title' })
              .text(this.title))
            .attr('title', this.title)
            .append($('<span />', { 'class' : 'feed-unread-count' }))
            .click(function() 
            {
              var subDom = $(this).closest('.feed');
              if (subDom.length > 0)
              {
                $('.feed.selected').removeClass('selected');
                subDom.addClass('selected');

                loadEntries(subDom);
              }
            }));

          $('#feeds').append(subDom);
      });
      
      $('#feeds').data('sub-map', subscriptionMap);
    });
  };

  var synchronizeEntryView = function(entryDom)
  {
    var entry = entryDom.data('entry');

    entryDom.toggleClass('star', $.inArray('star', entry.properties) > -1);
    entryDom.toggleClass('like', $.inArray('like', entry.properties) > -1);
    entryDom.toggleClass('read', $.inArray('unread', entry.properties) < 0);
  };

  var isPropertySet = function(entryDom, propertyName)
  {
    var entry = entryDom.data('entry');
    return $.inArray(propertyName, entry.properties) > -1;
  };

  var setProperty = function(entryDom, propertyName, propertyValue)
  {
    var entry = entryDom.data('entry');
    var isSet = isPropertySet(entryDom, propertyName);

    if (propertyValue == isSet)
      return; // Already set

    $.getJSON('setProperty', 
    {
      entry:        entry.id,
      subscription: entry.source,
      property:     propertyName,
      set:          propertyValue,
    },
    function(properties)
    {
      entry.properties = properties;
      synchronizeEntryView(entryDom);
    });
  };

  var toggleProperty = function(entryDom, propertyName)
  {
    var entry = entryDom.data('entry');
    var isSet = isPropertySet(entryDom, propertyName);

    setProperty(entryDom, propertyName, !isSet);
  };

  var loadEntries = function(subscriptionDom)
  {
    var subscriptionId = subscriptionDom.data('sub-id');

    $.getJSON('entries', 
    {
      subscription: subscriptionId,
    },
    function(entries)
    {
      $('#entries').empty();

      $.each(entries, function()
      {
        var entryDom = $('<div />', { 'class' : 'entry' });

        var entry = this;
        var subscription = getSubscription(entry.source);
        
        entryDom
          .data('entry', entry)
          .append($('<div />', { 'class' : 'entry-item' })
            .append($('<div />', { 'class' : 'action-star' })
              .click(function(e)
              {
                toggleProperty(entryDom, "star");
                e.stopPropagation();
              }))
            .append($('<span />', { 'class' : 'entry-source' }).text(subscription.title))
            .append($('<a />', { 'class' : 'entry-link', 'href' : entry.link, 'target' : '_blank' })
              .click(function(e)
              {
                e.stopPropagation();
              }))
            .append($('<span />', { 'class' : 'entry-pubDate' })
              .text(getPublishedDate(entry.published)))
            .append($('<div />', { 'class' : 'entry-excerpt' })
              .append($('<h2 />', { 'class' : 'entry-title' }).text(entry.title))))
          .click(function() 
          {
            // Unselect/reselect
            $('.entry.selected').removeClass('selected');
            entryDom.addClass('selected');

            var wasExpanded = entryDom.hasClass('open');

            // Collapse all
            $('.entry.open').removeClass('open');
            $('.entry .entry-content').remove();

            if (!wasExpanded)
              expandEntry(entryDom);
          });

        if (entry.summary)
        {
          entryDom.find('.entry-excerpt')
            .append($('<span />', { 'class' : 'entry-spacer' }).text(' - '))
            .append($('<span />', { 'class' : 'entry-summary' }).text(entry.summary));
        }

        synchronizeEntryView(entryDom);

        $('#entries').append(entryDom);
      });
    });
  };

  var expandEntry = function(entryDom)
  {
    if (entryDom.find('.entry-content').length > 0)
      return;

    if (isPropertySet(entryDom, 'unread'))
      setProperty(entryDom, 'unread', false);

    var entry = entryDom.data('entry');
    var subscription = getSubscription(entry.source);

    var content = 
      $('<div />', { 'class' : 'entry-content' })
        .append($('<div />', { 'class' : 'article' })
          .append($('<a />', { 'href' : entry.link, 'target' : '_blank', 'class' : 'article-title' })
            .append($('<h2 />')
              .text(entry.title)))
          .append($('<div />', { 'class' : 'article-author' })
            .append('from ')
            .append($('<a />', { 'href' : subscription.link, 'target' : '_blank' })
              .text(subscription.title)))
          .append($('<div />', { 'class' : 'article-body' })
            .append(entry.content)))
        .append($('<div />', { 'class' : 'entry-footer'})
          .append($('<span />', { 'class' : 'action-star' })
            .click(function(e)
            {
              toggleProperty(entryDom, "star");
            }))
          .append($('<span />', { 'class' : 'action-unread entry-action'})
            .text(_l('Keep unread'))
            .click(function(e)
            {
              toggleProperty(entryDom, "unread");
            }))
          // .append($('<span />', { 'class' : 'action-tag entry-action'})
          //   .text(entry.tags.length ? _l('Edit tags: %s', [ entry.tags.join(', ') ]) : _l('Add tags'))
          //   .toggleClass('has-tags', entry.tags.length > 0)
          //   .click(function(e)
          //   {
          //     editTags(entryDom);
          //   }))
          // .append($('<span />', { 'class' : 'action-like entry-action'})
          //   .text((entry.like_count < 1) ? _l('Like') : _l('Like (%s)', [entry.like_count]))
          //   .click(function(e)
          //   {
          //     toggleProperty(entryDom, "like");
          //   }))
        )
        .click(function(e)
        {
          e.stopPropagation();
        });

    if (entry.author)
      content.find('.article-author')
        .append(' by ')
        .append($('<span />')
          .text(entry.author));

    // Links in the content should open in a new window
    content.find('.article-body a').attr('target', '_blank');

    entryDom.toggleClass('open', true);
    entryDom.append(content);
  };

  loadSubscriptions();
});
