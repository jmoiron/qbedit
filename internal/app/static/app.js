// Simple toggles using Cash (jQuery-compatible)
$(function() {
  // Flash banner utility available globally
  (function(){
    function ensureFlash(){
      var el = document.getElementById('flash');
      if (!el) {
        el = document.createElement('div');
        el.id = 'flash';
        el.className = 'flash';
        el.style.display = 'none';
        // insert near top of main content if available, else body
        var main = document.querySelector('.main');
        if (main) main.prepend(el); else document.body.prepend(el);
      }
      return $(el);
    }
    function showFlash(msg, ok){
      var $f = ensureFlash();
      $f.text(msg || '').removeClass('ok fail').addClass(ok ? 'ok' : 'fail').show();
      setTimeout(function(){ $f.hide(); }, 5000);
    }
    window.showFlash = showFlash;
  })();

  // Theme init
  function applyTheme(theme){
    var root = document.documentElement;
    if(theme === 'dark') root.classList.add('dark');
    else root.classList.remove('dark');
    var link = document.getElementById('toggle-theme');
    if(link){ link.textContent = (theme === 'dark') ? 'Light mode' : 'Dark mode'; }
  }
  (function(){
    // Determine theme: query param overrides localStorage
    var params = new URLSearchParams(window.location.search);
    var qDark = params.get('dark');
    var th;
    if (qDark !== null) {
      th = (qDark === '1' || qDark === 'true' || qDark === 'yes' || qDark === 'on') ? 'dark' : 'light';
    } else {
      th = localStorage.getItem('theme') || 'light';
    }
    applyTheme(th);
    var link = document.getElementById('toggle-theme');
    if(link){
      link.addEventListener('click', function(e){
        e.preventDefault();
        var cur = (localStorage.getItem('theme') || 'light');
        var next = cur === 'dark' ? 'light' : 'dark';
        localStorage.setItem('theme', next);
        document.cookie = 'theme=' + next + '; Path=/; Max-Age=31536000; SameSite=Lax';
        applyTheme(next);
      });
    }
  })();

  const keyFor = (id) => 'grp:' + id;
  function setGroup(id, expand, persist=true) {
    var $list = $('[data-list="' + id + '"]');
    var $toggle = $('[data-toggle="' + id + '"]');
    if (expand) {
      $list.show();
      $toggle.text('[-]');
      if (persist) localStorage.setItem(keyFor(id), '1');
    } else {
      $list.hide();
      $toggle.text('[+]');
      if (persist) localStorage.setItem(keyFor(id), '0');
    }
  }

  // Initialize lists based on localStorage (default: expanded)
  $('[data-list]').each(function(_, el) {
    var id = $(el).attr('data-list');
    var st = localStorage.getItem(keyFor(id));
    if (st === '0') {
      setGroup(id, false, false);
    } else {
      setGroup(id, true, false);
    }
  });

  // Per-group toggle
  $(document).on('click', '.group-toggle', function(e) {
    e.preventDefault();
    var id = $(this).attr('data-toggle');
    var $list = $('[data-list="' + id + '"]');
    var expanded = $list.css('display') !== 'none';
    setGroup(id, !expanded);
  });

  // Expand/collapse all
  $(document).on('click', '.toggle-all', function(e) {
    e.preventDefault();
    var action = $(this).attr('data-action');
    var expand = action === 'expand-all';
    $('[data-list]').each(function(_, el) {
      var id = $(el).attr('data-list');
      setGroup(id, expand);
    });
  });
});
