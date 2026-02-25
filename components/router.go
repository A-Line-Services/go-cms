package components

import (
	"encoding/json"

	cms "go.a-line.be/cms"
)

// routerScript builds the complete inline <script> tags for the SPA router.
// Returns empty string when no layouts are registered.
func routerScript(p cms.PageData) string {
	manifest := p.LayoutManifest()
	if len(manifest) == 0 {
		return ""
	}

	cfg := map[string]any{
		"layouts":      manifest,
		"localePrefix": p.LocalePrefix(),
	}

	// Include locale metadata so the router can detect cross-locale
	// navigation and update the language switcher after SPA navigations.
	if len(p.Locales) > 1 {
		codes := make([]string, len(p.Locales))
		var dl string
		for i, l := range p.Locales {
			codes[i] = l.Code
			if l.IsDefault {
				dl = l.Code
			}
		}
		cfg["locales"] = codes
		cfg["defaultLocale"] = dl
	}

	data, _ := json.Marshal(cfg)

	return `<script id="__cms_router" type="application/json">` + string(data) + `</script>` +
		`<script type="module">` + routerJS + `</script>`
}

// routerJS is the self-contained SPA router that:
//   - Intercepts internal link clicks for SPA-like navigation
//   - Prefetches page fragments on hover / touchstart
//   - Determines the deepest common layout and fetches only the needed fragment
//   - Swaps content at the correct [data-layout] boundary
//   - Uses View Transitions API where available
//   - Updates document title, active nav, and browser history
//   - Dispatches "cms:navigate" events for per-page JS hooks
//   - Degrades gracefully without JS (every page is full static HTML)
const routerJS = `(function(){
var d=document,w=window,h=history;
var cfg=JSON.parse(d.getElementById("__cms_router").textContent);
var L=cfg.layouts,LP=cfg.localePrefix||"";
var locales=cfg.locales||[],DL=cfg.defaultLocale||"";
var cache=new Map(),cur=location.pathname;

function strip(p){
if(LP&&p.startsWith(LP+"/")){return p.slice(LP.length)||"/"}
if(LP&&p===LP){return"/"}
return p;
}

function crossLocale(path){
if(!locales.length)return false;
for(var i=0;i<locales.length;i++){
var lp="/"+locales[i];
if(lp!==LP&&(path===lp||path.startsWith(lp+"/")))return true;
}
if(LP&&path!==LP&&!path.startsWith(LP+"/"))return true;
return false;
}

function altPath(cp,lc){
if(lc===DL)return cp;
return cp==="/"?"/"+lc:"/"+lc+cp;
}

function updateLangSwitcher(path){
if(!locales.length)return;
var cp=strip(path);
d.querySelectorAll(".lang-switcher a").forEach(function(a){
var lc=a.textContent.trim().toLowerCase();
a.href=altPath(cp,lc);
});
}

function chain(path){
var p=strip(path),c=[];
for(var px in L){
if(px==="/"||p===px||p.startsWith(px+"/")){c.push([px.length,L[px]])}
}
c.sort(function(a,b){return a[0]-b[0]});
return c.map(function(x){return x[1]});
}

function common(a,b){
var r=a[0];
for(var i=0;i<Math.min(a.length,b.length);i++){
if(a[i]===b[i])r=a[i];else break;
}
return r;
}

function fragURL(path,lid){
if(path==="/"||path.endsWith("/")){return path+"_"+lid+".html"}
return path+"/_"+lid+".html";
}

function prefetch(url){
try{
var p=new URL(url,location.origin).pathname;
if(crossLocale(p))return;
var lid=common(chain(cur),chain(p));
var fu=fragURL(p,lid);
if(!cache.has(fu)){
cache.set(fu,fetch(fu).then(function(r){
if(!r.ok)throw new Error(r.status);return r.text();
}).catch(function(){cache.delete(fu);return null}));
}
}catch(e){}
}

function parseFrag(html){
var m=html.match(/^<!--route:(\{.*?\})-->\n?/);
if(m){return{meta:JSON.parse(m[1]),html:html.slice(m[0].length)}}
return{meta:{},html:html};
}

function navigate(url,push){
var p=new URL(url,location.origin).pathname;
if(p===cur&&!location.hash)return Promise.resolve();

var fromC=chain(cur),toC=chain(p);
var lid=common(fromC,toC);
var fu=fragURL(p,lid);

var prom=cache.has(fu)?cache.get(fu):fetch(fu).then(function(r){
if(!r.ok)throw new Error(r.status);return r.text();
});

return prom.then(function(raw){
if(!raw){location.href=url;return}
var f=parseFrag(raw);
var target=d.querySelector('[data-layout="'+lid+'"]');
if(!target){location.href=url;return}

var swap=function(){
target.innerHTML=f.html;
target.querySelectorAll("script").forEach(function(old){
var s=d.createElement("script");
for(var i=0;i<old.attributes.length;i++){
s.setAttribute(old.attributes[i].name,old.attributes[i].value);
}
s.textContent=old.textContent;
old.replaceWith(s);
});
};

if(d.startViewTransition){d.startViewTransition(swap)}else{swap()}
if(push!==false)h.pushState({},"",url);
cur=p;
if(f.meta.t)d.title=f.meta.t;
d.querySelectorAll("a[href]").forEach(function(a){
try{
var hp=new URL(a.href,location.origin).pathname;
if(hp===p)a.setAttribute("aria-current","page");
else a.removeAttribute("aria-current");
}catch(e){}
});
updateLangSwitcher(p);
w.dispatchEvent(new CustomEvent("cms:navigate",{detail:{path:p}}));
w.scrollTo(0,0);
}).catch(function(){location.href=url});
}

d.addEventListener("click",function(e){
var a=e.target.closest("a[href]");
if(!a||e.ctrlKey||e.metaKey||e.shiftKey||a.target==="_blank")return;
try{
var u=new URL(a.href,location.origin);
if(u.origin!==location.origin)return;
if(u.pathname===cur&&!u.hash)return;
if(crossLocale(u.pathname))return;
}catch(x){return}
e.preventDefault();
navigate(a.href);
});

d.addEventListener("mouseenter",function(e){
var a=e.target&&e.target.closest&&e.target.closest("a[href]");
if(!a)return;
try{var u=new URL(a.href,location.origin);if(u.origin===location.origin)prefetch(a.href)}catch(x){}
},{capture:true,passive:true});

d.addEventListener("touchstart",function(e){
var a=e.target&&e.target.closest&&e.target.closest("a[href]");
if(!a)return;
try{var u=new URL(a.href,location.origin);if(u.origin===location.origin)prefetch(a.href)}catch(x){}
},{capture:true,passive:true});

w.addEventListener("popstate",function(){
cur=location.pathname;
navigate(location.href,false);
});
})();`
