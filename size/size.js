/**
 * Size Plot Interactions.
 */
(function() {
  /**
   * $$ returns a real JS array of DOM elements that match the CSS query selector.
   *
   * A shortcut for jQuery-like $ behavior.
   **/
  function $$(query, ele) {
    if (!ele) {
      ele = document;
    }
    return Array.prototype.map.call(ele.querySelectorAll(query), function(e) { return e; });
  }

  /**
   * $$$ returns the DOM element that match the CSS query selector.
   *
   * A shortcut for document.querySelector.
   **/
  function $$$(query, ele) {
    if (!ele) {
      ele = document;
    }
    return ele.querySelector(query);
  }
  
  get = function(url) {
    // Return a new promise.
    return new Promise(function(resolve, reject) {
      // Do the usual XHR stuff
      var req = new XMLHttpRequest();
      req.open('GET', url);

      req.onload = function() {
        // This is called even on 404 etc
        // so check the status
        if (req.status == 200) {
          // Resolve the promise with the response text
          resolve(req.response);
        } else {
          // Otherwise reject with the status text
          // which will hopefully be a meaningful error
          reject(req.response);
        }
      };

      // Handle network errors
      req.onerror = function() {
        reject(Error("Network Error"));
      };

      // Make the request
      req.send();
    });
  }
  
  function onLoad() {
    var j = get('http://skiagit.appspot.com/sizedata');
    $$$('#plot').innerText = j;
  }
  
  if (document.readyState != 'loading') {
    onLoad();
  } else {
    window.addEventListener('load', onLoad);
  }
})();
