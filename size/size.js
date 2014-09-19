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
  
  function onLoad() {
    $$$('#plot').innerText = 'Here goes plot.';
  }
  
  if (document.readyState != 'loading') {
    onLoad();
  } else {
    window.addEventListener('load', onLoad);
  }
})();
