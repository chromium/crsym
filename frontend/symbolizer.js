/* Copyright 2013 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/


(function main() {
  document.addEventListener('DOMContentLoaded', function() {
    var controller = new SymbolizerController();
  });
})();

/**
 * SymbolizerController is a view controller for the crsym frontend.
 * @constructor
 */
var SymbolizerController = SymbolizerController || function() {
  /**
   * The name of the input type, specified in the data-input-type on the
   * input_option div.
   * @type string
   */
  this.type_ = null;

  /**
   * The form element that contains all the input fields.
   * @type HTMLFormElement
   */
  this.form_ = document.getElementById('form');

  /**
   * A collection of all the input options.
   * @type HTMLCollection.<HTMLDivElement>
   */
  this.inputTypes_ = document.getElementsByClassName('input_option');

  this.fragmentData_ = document.getElementById('fragment_data');
  this.crashKeyData_ = document.getElementById('crash_key_data');
  this.moduleInfoData_ = document.getElementById('module_info_data');
  this.androidData_ = document.getElementById('android_data');
  this.output_ = document.getElementById('output');

  document.getElementById('symbolize').onclick = this.onSymbolize.bind(this);

  for (var i = 0; i < this.inputTypes_.length; ++i) {
    this.inputTypes_[i].onclick = this.onInputOptionClicked.bind(this);
  }

  var tabs = document.getElementById('tabs');
  for (var i = 0; i < tabs.children.length; ++i) {
    var tab = tabs.children[i];
    tab.onclick = this.onTabClicked.bind(this);
  }

  // Initial view layout.
  this.changeInputType_('apple');
  this.switchToTab_('input');
};

/**
 * Click event handler for the input type option.
 * @param {Event} e The click event.
 */
SymbolizerController.prototype.onInputOptionClicked = function(e) {
  // Find the top-level div for the input_option.
  var option = e.target;
  while (option && option.classList &&
         !option.classList.contains('input_option')) {
    option = option.parentNode;
  }

  if (!option || !option.classList ||
      !option.classList.contains('input_option')) {
    throw 'Could not find option for ' + e;
  }

  // Now find the radio button for it, which is what controls the value.
  this.changeInputType_(option.dataset.inputType);
};

/**
 * Updates the UI, hiding and showing any necessary elements, in response to
 * a change in the input type.
 * @param {string} newType The name of the input type for which the UI should
 *                         upate.
 */
SymbolizerController.prototype.changeInputType_ = function(newType) {
  // Update the input type selection.
  this.type_ = newType;
  for (var i = 0; i < this.inputTypes_.length; ++i) {
    var input = this.inputTypes_[i];
    input.classList.remove('active');

    if (input.dataset.inputType == newType) {
      input.firstElementChild.checked = true;  // Check the radio button.
      input.classList.add('active');
    }
  }

  // Hide the appropriate additional input fields.
  this.fragmentData_.hidden = true;
  this.crashKeyData_.hidden = true;
  this.moduleInfoData_.hidden = true;
  this.androidData_.hidden = true;

  var inputTab = this.form_.querySelector('label[for=input]');
  inputTab.style.display = '';

  if (this.type_ == 'fragment') {
    this.fragmentData_.hidden = false;
    this.switchToTab_('input');
  } else if (this.type_ == 'apple') {
    this.switchToTab_('input');
  } else if (this.type_ == 'stackwalk') {
    this.switchToTab_('input');
  } else if (this.type_ == 'crash_key') {
    this.crashKeyData_.hidden = false;
    inputTab.style.display = 'none';
    this.switchToTab_('output');
  } else if (this.type_ == 'module_info') {
    this.moduleInfoData_.hidden = false;
    inputTab.style.display = 'none';
    this.switchToTab_('output');
  } else if (this.type_ == 'android') {
    this.androidData_.hidden = false;
    this.switchToTab_('input');
  }
};

/**
 * Event handler for switching between tabs.
 * @param {Event} e
 */
SymbolizerController.prototype.onTabClicked = function(e) {
  this.switchToTab_(e.target.htmlFor);
};

/**
 * Switches to a tab with the given name.
 * @param {string} name
 */
SymbolizerController.prototype.switchToTab_ = function(name) {
  var allTabs = document.getElementById('tab_contents').children;
  for (var i = 0; i < allTabs.length; ++i) {
    var tab = allTabs[i];
    tab.classList.remove('active');

    var label = tab.labels[0];
    label.classList.remove('active');

    if (tab.id == name) {
      label.classList.add('active');
      tab.classList.add('active');
      tab.focus();
    }
  }
};

/**
 * Event handler for the Symbolize submit button, which sends the actual
 * request to the server.
 */
SymbolizerController.prototype.onSymbolize = function() {
  var xhr = new XMLHttpRequest();
  xhr.onreadystatechange = this.onXhrStateChange_.bind(this);
  xhr.open('POST', '/_/service', /*async=*/true);
  xhr.setRequestHeader('Content-type', 'application/x-www-form-urlencoded');

  var data = ['input_type=' + this.type_];
  var elements = this.form_.elements;
  for (var i = 0; i < elements.length; ++i) {
    var elm = elements[i];
    if (!elm.name || elm.name == 'input_type')
      continue;
    data.push(elm.name + '=' + encodeURIComponent(elm.value));
  }
  xhr.send(data.join('&'));

  // The backend may take a few seconds to respond, so update the UI
  // immediately to make it look like something is happening.
  this.switchToTab_('output');
  this.output_.classList.remove('error');
  this.output_.classList.add('in_progress');
  this.output_.value = 'Processing\u2026\n\nThis may take up to 60 seconds.';
};

/**
 * Ready state change handler for the symbolization request.
 * @param {XMLHttpRequestProgressEvent} e
 * @private
 */
SymbolizerController.prototype.onXhrStateChange_ = function(e) {
  var xhr = e.target;
  if (xhr.readyState != 4 /*DONE*/)
    return;

  this.output_.classList.remove('in_progress');

  if (xhr.status == 200)
    this.output_.classList.remove('error');
  else
    this.output_.classList.add('error');

  this.output_.value = xhr.responseText;

  this.switchToTab_('output');
};
