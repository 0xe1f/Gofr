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
 
$().ready(function()
{
  var subscriptionMap = null;

  $('button.refresh').click(function()
  {
    loadSubscriptions();
  });

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

  var subscriptionMethods = 
  {
    'getDom': function() 
    {
      return $('#subscriptions').find('.' + this.domId);
    },
    'loadEntries': function()
    {
      var subscription = this;

      $.getJSON('entries', 
      {
        subscription: subscription.id,
      },
      function(entries)
      {
        $('#entries').empty();

        var idCounter = $('#entries').find('.entry').length;

        $.each(entries, function()
        {
          var entry = this;

          // Inject methods
          for (var name in entryMethods)
            entry[name] = entryMethods[name];

          var entrySubscription = entry.getSubscription();

          entry.domId = 'entry-' + idCounter++;
          var entryDom = $('<div />', { 'class' : 'entry ' + entry.domId})
            .data('entry', entry)
            .append($('<div />', { 'class' : 'entry-item' })
              .append($('<div />', { 'class' : 'action-star' })
                .click(function(e)
                {
                  entry.toggleProperty("star");
                  e.stopPropagation();
                }))
              .append($('<span />', { 'class' : 'entry-source' })
                .text(entrySubscription.title))
              .append($('<a />', { 'class' : 'entry-link', 'href' : entry.link, 'target' : '_blank' })
                .click(function(e)
                {
                  e.stopPropagation();
                }))
              .append($('<span />', { 'class' : 'entry-pubDate' })
                .text(getPublishedDate(entry.published)))
              .append($('<div />', { 'class' : 'entry-excerpt' })
                .append($('<h2 />', { 'class' : 'entry-title' })
                  .text(entry.title))))
            .click(function() 
            {
              entry.select();
              
              var wasExpanded = entry.isExpanded();

              collapseAllEntries();
              if (!wasExpanded)
                entry.expand();
            });

          if (entry.summary)
          {
            entryDom.find('.entry-excerpt')
              .append($('<span />', { 'class' : 'entry-spacer' }).text(' - '))
              .append($('<span />', { 'class' : 'entry-summary' }).text(entry.summary));
          }

          $('#entries').append(entryDom);

          entry.syncView();
        });
      });
    },
    'select': function()
    {
      $('#subscriptions').find('.subscription.selected').removeClass('selected');
      this.getDom().addClass('selected');

      this.loadEntries();
    },
    'syncView': function()
    {
      var feedDom = this.getDom();

      feedDom.find('.subscription-unread-count').text('(' + this.unread + ')');
      feedDom.find('.subscription-item').toggleClass('has-unread', this.unread > 0);
    },
  };

  var entryMethods = 
  {
    'getSubscription': function() 
    {
      return subscriptionMap[this.source];
    },
    'getDom': function() 
    {
      return $('#entries').find('.' + this.domId);
    },
    'hasProperty': function(propertyName)
    {
      return $.inArray(propertyName, this.properties) > -1;
    },
    'setProperty': function(propertyName, propertyValue)
    {
      if (propertyValue == this.hasProperty(propertyName))
        return; // Already set

      var entry = this;

      $.getJSON('setProperty', 
      {
        entry:        this.id,
        subscription: this.source,
        property:     propertyName,
        set:          propertyValue,
      },
      function(properties)
      {
        delete entry.properties;

        entry.properties = properties;
        entry.syncView();

        if (propertyName == 'read')
        {
          var subscription = entry.getSubscription();
          if (propertyValue)
            subscription.unread -= 1;
          else
            subscription.unread += 1;

          subscription.syncView();
        }
      });
    },
    'toggleProperty': function(propertyName)
    {
      this.setProperty(propertyName, 
        !this.hasProperty(propertyName));
    },
    'syncView': function()
    {
      this.getDom()
        .toggleClass('star', this.hasProperty('star'))
        .toggleClass('like', this.hasProperty('like'))
        .toggleClass('read', this.hasProperty('read'));
    },
    'isExpanded': function()
    {
      return this.getDom().hasClass('open');
    },
    'expand': function()
    {
      var entry = this;
      var subscription = this.getSubscription();
      var entryDom = this.getDom();

      if (this.isExpanded())
        return;

      if (!this.hasProperty('read'))
        this.setProperty('read', true);

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
                entry.toggleProperty("star");
              }))
            .append($('<span />', { 'class' : 'action-unread entry-action'})
              .text(_l('Keep unread'))
              .click(function(e)
              {
                entry.toggleProperty("read");
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

      if (this.author)
        content.find('.article-author')
          .append(' by ')
          .append($('<span />')
            .text(entry.author));

      // Links in the content should open in a new window
      content.find('.article-body a').attr('target', '_blank');

      entryDom.toggleClass('open', true);
      entryDom.append(content);
    },
    'collapse': function()
    {
      this.getDom()
        .removeClass('open')
        .find('.entry-content')
          .remove();
    },
    'select': function()
    {
      $('#entries').find('.entry.selected').removeClass('selected');
      this.getDom().addClass('selected');
    },
  };

  var collapseAllEntries = function()
  {
    $('.entry.open').removeClass('open');
    $('.entry .entry-content').remove();
  };

  var loadSubscriptions = function()
  {
    $.getJSON('subscriptions', 
    {
    },
    function(subscriptions)
    {
      var selectedSubscriptionId = null;
      if ($('.subscription.selected').length > 0)
        selectedSubscriptionId = $('.subscription.selected').data('subscription').id;

      $('#subscriptions').empty();

      if (subscriptionMap != null)
        delete subscriptionMap;

      var idCounter = 0;
      subscriptionMap = {};

      $.each(subscriptions, function()
      {
        var subscription = this;
        subscription.domId = 'sub-' + idCounter++;

        // Inject methods
        for (var name in subscriptionMethods)
          subscription[name] = subscriptionMethods[name];

        var subDom = $('<li />', { 'class' : 'subscription ' + subscription.domId })
          .data('subscription', subscription)
          .append($('<div />', { 'class' : 'subscription-item' })
            .append($('<span />', { 'class' : 'chevron' })
              .click(function(e)
              {
                // FIXME: show the menu
                e.stopPropagation();
              }))
            .append($('<div />', { 'class' : 'subscription-icon' }))
            .append($('<span />', { 'class' : 'subscription-title' })
              .text(subscription.title))
            .attr('title', subscription.title)
            .append($('<span />', { 'class' : 'subscription-unread-count' }))
            .click(function() 
            {
              subscription.select();
            }));

          $('#subscriptions').append(subDom);

          subscriptionMap[subscription.id] = subscription;
          subscription.syncView();

          if (subscription.id == selectedSubscriptionId)
            subscription.select();
      });
    });
  };

  loadSubscriptions();
});
