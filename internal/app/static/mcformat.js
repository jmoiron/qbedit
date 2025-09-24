(function(global){
  function escapeHTML(s){
    return s.replace(/[&<>"']/g, function(c){
      switch(c){
        case '&': return '&amp;';
        case '<': return '&lt;';
        case '>': return '&gt;';
        case '"': return '&quot;';
        case "'": return '&#39;';
      }
      return c;
    });
  }
  function mcFormat(input){
    if(input == null) return '';
    var s = String(input);
    var st = {color:'', bold:false, italic:false, underline:false, strike:false, obf:false};
    var out = '';
    function open(){
      var classes = ['mc-text'];
      if(st.color) classes.push('mc-' + st.color);
      if(st.bold) classes.push('mc-bold');
      if(st.italic) classes.push('mc-italic');
      if(st.underline) classes.push('mc-underline');
      if(st.strike) classes.push('mc-strike');
      if(st.obf) classes.push('mc-obf');
      out += '<span class="' + classes.join(' ') + '">';
    }
    var openSpan = false;
    function close(){ if(openSpan){ out += '</span>'; openSpan=false; } }
    function reset(){ st = {color:'', bold:false, italic:false, underline:false, strike:false, obf:false}; }
    function setColor(c){
      var map = {
        '0':'c0','1':'c1','2':'c2','3':'c3','4':'c4','5':'c5','6':'c6','7':'c7',
        '8':'c8','9':'c9','a':'ca','A':'ca','b':'cb','B':'cb','c':'cc','C':'cc',
        'd':'cd','D':'cd','e':'ce','E':'ce','f':'cf','F':'cf'
      };
      st.color = map[c] || st.color;
    }
    for(var i=0;i<s.length;i++){
      var r = s[i];
      if((r==='§' || r==='&') && i+1 < s.length){
        var code = s[i+1];
        switch(code){
          case 'k': case 'K': close(); st.obf=true; open(); openSpan=true; i++; continue;
          case 'l': case 'L': close(); st.bold=true; open(); openSpan=true; i++; continue;
          case 'm': case 'M': close(); st.strike=true; open(); openSpan=true; i++; continue;
          case 'n': case 'N': close(); st.underline=true; open(); openSpan=true; i++; continue;
          case 'o': case 'O': close(); st.italic=true; open(); openSpan=true; i++; continue;
          case 'r': case 'R': close(); reset(); i++; continue;
          default: close(); setColor(code); open(); openSpan=true; i++; continue;
        }
      }
      if(!openSpan){ open(); openSpan=true; }
      out += escapeHTML(r);
    }
    close();
    return out;
  }
  global.mcFormat = mcFormat;
})(window);

