// js/main.js

/* ---- Language ---- */

const LANG_KEY = 'shop_lang';

function detectLang(langs) {
  const fromUrl = new URLSearchParams(window.location.search).get('lang');
  if (langs.includes(fromUrl)) return fromUrl;
  const saved = localStorage.getItem(LANG_KEY);
  if (langs.includes(saved)) return saved;
  const browser = (navigator.language || 'en').slice(0, 2).toLowerCase();
  // ru locale also maps to Ukrainian — intentional for target audience
  const mapped = ['uk', 'ru'].includes(browser) ? 'uk' : 'en';
  return langs.includes(mapped) ? mapped : (langs[0] || 'en');
}

// On the product page, reflect the active language into the URL so a copied
// link previews and opens in the language the sharer was viewing. No-op elsewhere.
function syncProductLangUrl() {
  if (!document.getElementById('product-view')) return;
  const p = new URLSearchParams(window.location.search);
  if (p.get('lang') === currentLang) return;
  p.set('lang', currentLang);
  history.replaceState(null, '', `?${p.toString()}`);
}

// Reflect the active carousel slide into the URL (?img=<index>) so a copied link
// reopens on the same image. Index is into product.images (same index attr_images/
// _goTo use). Omitted at 0 (the default) to keep URLs clean.
function syncCarouselUrl(index) {
  const p = new URLSearchParams(window.location.search);
  if (index > 0) p.set('img', index);
  else p.delete('img');
  history.replaceState(null, '', `?${p.toString()}`);
}

let availableLangs = [];
let currentLang = 'en'; // resolved after fetchShop() in DOMContentLoaded

function t(key) {
  return (I18N[currentLang] && I18N[currentLang][key]) || key;
}

function selectPrice(product, lang) {
  const map = product.price;
  // GET /products/{id}/{lang} returns an already-localised {value, currency} object
  if (typeof map.value === 'number') return map;
  if (lang === 'uk' && map.ua) return map.ua;
  return map.default || Object.values(map)[0];
}

function formatPrice(priceItem) {
  const n = Math.round(priceItem.value);
  if (priceItem.currency === 'UAH') {
    return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, '\u00a0') + '\u00a0₴';
  }
  if (priceItem.currency === 'USD') return '$' + n.toLocaleString('en-US');
  if (priceItem.currency === 'EUR') return '€' + n.toLocaleString('en-US');
  return n + '\u00a0' + priceItem.currency;
}

function esc(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function productTagline(product, lang) {
  const desc = (product.description && product.description[lang]) || '';
  return desc.split('\n')[0];
}

// Localised string from a {lang: text} map: the active language, else the first
// available one, else ''.
function loc(map) {
  if (!map || typeof map !== 'object') return '';
  return map[currentLang] || Object.values(map)[0] || '';
}

function assetUrl(name) {
  return `${API_BASE}/assets/${encodeURIComponent(name)}`;
}

async function applyShopContent() {
  try {
    const shop = await fetchShop();
    const name = loc(shop.name) || loc(shop.title);
    const desc = loc(shop.description);
    // Only the home page is titled by the shop name; other views set their own title.
    if (name && !getPageId()) document.title = name;
    if (desc) {
      const meta = document.querySelector('meta[name="description"]');
      if (meta) meta.content = desc;
    }
  } catch (e) {
    // silently fail — static fallback remains
  }
}

// Load Google Analytics (gtag.js) when the /shop response carries
// `google_analytics.id` (a GA4 measurement id, e.g. "G-XXXXXXXXXX"). Kept out of
// the static HTML so the published code ships no specific id — the backend
// supplies one per deployment. No-op when absent or after the first call.
let _analyticsLoaded = false;
function applyAnalytics() {
  if (_analyticsLoaded) return;
  const shop = getCachedShop();
  const id = shop && shop.google_analytics && shop.google_analytics.id;
  if (!id) return;
  _analyticsLoaded = true;
  const s = document.createElement('script');
  s.async = true;
  s.src = `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(id)}`;
  document.head.appendChild(s);
  window.dataLayer = window.dataLayer || [];
  window.gtag = function gtag() { window.dataLayer.push(arguments); };
  window.gtag('js', new Date());
  window.gtag('config', id);
}

// Static assets (favicon, touch icon, footer logo) are served by the backend at
// `/assets/<file>` — none are bundled in the published frontend. Wired up from
// `API_BASE` once at startup; idempotent. Filenames are the conventional ones
// the backend exposes.
function applyAssets() {
  const icons = [
    { rel: 'icon', href: assetUrl('favicon.svg'), type: 'image/svg+xml' },
    { rel: 'icon', href: assetUrl('favicon.ico'), sizes: 'any' },
    { rel: 'apple-touch-icon', href: assetUrl('apple-touch-icon.png') },
  ];
  icons.forEach(spec => {
    const link = document.createElement('link');
    Object.entries(spec).forEach(([k, v]) => link.setAttribute(k, v));
    document.head.appendChild(link);
  });
}

/* ---- Icons ---- */

// Stroke icons (24×24 grid, currentColor). Feature icons are keyed by
// ShopFeature.icon from /shop; an unknown name renders nothing.
const ICON_PATHS = {
  instagram: '<rect x="3" y="3" width="18" height="18" rx="5"/><circle cx="12" cy="12" r="4"/><circle cx="17.5" cy="6.5" r="0.6" fill="currentColor"/>',
  facebook: '<path d="M15 3h-2a4 4 0 0 0-4 4v3H7v4h2v7h4v-7h3l1-4h-4V7a1 1 0 0 1 1-1h2z"/>',
  telegram: '<path d="M21 4 3 11l6 2 2 6 3-4 5 4z"/><path d="m9 13 8-6"/>',
  youtube: '<rect x="2" y="5" width="20" height="14" rx="4"/><path d="m10 9 5 3-5 3z"/>',
  tiktok: '<path d="M14 3v11a4 4 0 1 1-4-4"/><path d="M14 3a5 5 0 0 0 5 5"/>',
  threads: '<path d="M17.5 8C16.5 6 14.6 4.5 12 4.5 7.8 4.5 5 7.6 5 12s2.8 7.5 7 7.5c3.4 0 5.8-2 5.8-4.8 0-2.8-2.5-4.2-5.3-4.2-2 0-3.5 1-3.5 2.6 0 1.4 1.1 2.3 2.6 2.3 2.4 0 3.6-1.8 3.6-5"/>',
  hand: '<path d="M18 11V6a2 2 0 0 0-4 0v5"/><path d="M14 10V4a2 2 0 0 0-4 0v6"/><path d="M10 10.5V6a2 2 0 0 0-4 0v8"/><path d="M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-6-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  box: '<path d="M21 8l-9-5-9 5v8l9 5 9-5z"/><path d="M3 8l9 5 9-5"/><path d="M12 13v8"/>',
  truck: '<path d="M3 6h11v10H3z"/><path d="M14 10h4l3 3v3h-7"/><circle cx="7" cy="18" r="2"/><circle cx="17" cy="18" r="2"/>',
  mail: '<rect x="3" y="5" width="18" height="14" rx="1"/><path d="m3 7 9 6 9-6"/>',
  shield: '<path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z"/><path d="m9 12 2 2 4-4"/>',
  heart: '<path d="M12 20s-7-4.5-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.5-7 10-7 10z"/>',
  star: '<path d="m12 3 2.8 5.6 6.2.9-4.5 4.4 1 6.1L12 17l-5.5 3 1-6.1L3 9.5l6.2-.9z"/>',
  gift: '<rect x="3" y="8" width="18" height="4"/><path d="M5 12v9h14v-9"/><path d="M12 8v13"/><path d="M12 8S10 3 7.5 4.5 9 8 12 8zm0 0s2-5 4.5-3.5S15 8 12 8z"/>',
  leaf: '<path d="M5 19c8 0 14-6 14-14-8 0-14 6-14 14z"/><path d="M5 19 13 11"/>',
  menu: '<path d="M4 7h16"/><path d="M4 12h16"/><path d="M4 17h16"/>',
  close: '<path d="M6 6l12 12"/><path d="M18 6 6 18"/>',
  chevronLeft: '<path d="m15 5-7 7 7 7"/>',
  chevronRight: '<path d="m9 5 7 7-7 7"/>',
};

function icon(name, size = 24) {
  const paths = ICON_PATHS[name];
  if (!paths) return '';
  return `<svg class="icon" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths}</svg>`;
}

/* ---- Header & Footer ---- */

const LANG_LABELS = { uk: 'UA' };
function langLabel(lang) {
  return LANG_LABELS[lang] || lang.toUpperCase();
}

function langSwitchHtml() {
  if (availableLangs.length < 2) return '';
  return `<div class="lang-switch" role="group" aria-label="${esc(t('nav.lang'))}">${availableLangs.map((lang, i) =>
    (i > 0 ? '<span class="lang-sep" aria-hidden="true">/</span>' : '') +
    `<a href="#" class="lang-link${lang === currentLang ? ' active' : ''}" data-lang="${esc(lang)}" lang="${esc(lang)}">${esc(langLabel(lang))}</a>`
  ).join('')}</div>`;
}

function socialLinks() {
  const shop = getCachedShop();
  return (shop && Array.isArray(shop.links) && shop.links) || [];
}

// Header nav: one link per shop category (catalog filtered by ?category=), falling back
// to a single catalog link when there are none, plus the content pages listed in
// shop.menu (titles come from /pages; ids missing there are skipped).
function navItems(pages) {
  const shop = getCachedShop() || {};
  const path = window.location.pathname.replace(/^\/|\/$/g, '');
  const categories = Array.isArray(shop.categories) ? shop.categories : [];
  const items = categories.length
    ? categories.map(c => ({
        href: `/catalog?category=${encodeURIComponent(c.id)}`,
        label: loc(c.title) || c.id,
        active: path === 'catalog' && activeCategory === c.id,
      }))
    : [{ href: '/catalog', label: t('nav.products'), active: path === 'catalog' || path === 'product' || path === 'product.html' }];
  (shop.menu || []).forEach(id => {
    const page = (pages || []).find(p => p.id === id);
    if (!page) return;
    items.push({ href: `/${id}`, label: loc(page.title) || id, active: path === id });
  });
  return items;
}

// Marquee: two identical groups so a -50% shift loops seamlessly. The copies are
// aria-hidden; screen readers (and reduced-motion users) get the static text.
// With href the whole bar is one link (shop.announcement_page).
const ANNOUNCEMENT_REPEAT = 8;

function announcementHtml(text, href) {
  const items = `<span class="announcement-item">${esc(text)}</span>`.repeat(ANNOUNCEMENT_REPEAT);
  const tag = href ? 'a' : 'div';
  return `
    <${tag} class="announcement"${href ? ` href="${esc(href)}"` : ''}>
      <span class="announcement-static">${esc(text)}</span>
      <div class="announcement-track" aria-hidden="true">
        <div class="announcement-group">${items}</div>
        <div class="announcement-group">${items}</div>
      </div>
    </${tag}>`;
}

async function renderHeader() {
  const el = document.getElementById('site-header');
  if (!el) return;
  const shop = getCachedShop() || {};
  let pages = [];
  try { pages = await fetchPages(); } catch (e) { /* nav falls back to the catalog link */ }
  const name = loc(shop.name) || loc(shop.title);
  const announcement = loc(shop.announcement);
  // Link only to a page /pages actually serves, so a stale id never leads to a 404.
  const announcementPage = shop.announcement_page && pages.find(p => p.id === shop.announcement_page);
  const nav = navItems(pages).map(i =>
    `<a href="${esc(i.href)}"${i.active ? ' class="active" aria-current="page"' : ''}>${esc(i.label)}</a>`
  ).join('');
  el.innerHTML = `
    ${announcement ? announcementHtml(announcement, announcementPage && `/${announcementPage.id}`) : ''}
    <header class="site-header">
      <div class="container header-inner">
        <button type="button" class="icon-link menu-toggle" aria-expanded="false" aria-controls="mobile-menu" aria-label="${esc(t('nav.menu'))}">${icon('menu', 22)}</button>
        <a href="/" class="wordmark">${esc(name)}</a>
        <nav class="main-nav" aria-label="${esc(t('nav.menu'))}">${nav}</nav>
        <div class="header-tools">${langSwitchHtml()}</div>
      </div>
      <div class="mobile-menu" id="mobile-menu" hidden>
        <nav class="container mobile-nav">${nav}</nav>
        <div class="container mobile-tools">${langSwitchHtml()}</div>
      </div>
    </header>
  `;
  const toggle = el.querySelector('.menu-toggle');
  const menu = el.querySelector('.mobile-menu');
  toggle.addEventListener('click', () => {
    const open = menu.hidden;
    menu.hidden = !open;
    toggle.setAttribute('aria-expanded', String(open));
    toggle.setAttribute('aria-label', t(open ? 'nav.close' : 'nav.menu'));
    toggle.innerHTML = icon(open ? 'close' : 'menu', 22);
  });
  bindLangLinks(el);
}

async function renderFooter() {
  const el = document.getElementById('site-footer');
  if (!el) return;
  const shop = getCachedShop() || {};
  let pages = [];
  try { pages = await fetchPages(); } catch (e) { /* info column stays empty */ }
  const name = loc(shop.name) || loc(shop.title);
  const categories = Array.isArray(shop.categories) ? shop.categories : [];
  const shopLinks = categories.length
    ? categories.map(c => `<a href="/catalog?category=${encodeURIComponent(c.id)}">${esc(loc(c.title) || c.id)}</a>`).join('')
    : `<a href="/catalog">${esc(t('nav.products'))}</a>`;
  const pageLinks = pages.map(p => `<a href="/${esc(p.id)}">${esc(loc(p.title) || p.id)}</a>`).join('');
  const social = socialLinks().map(l =>
    `<a href="${esc(l.url)}" target="_blank" rel="noopener">${esc(l.title)}</a>`
  ).join('');
  el.innerHTML = `
    <footer class="site-footer">
      <div class="container">
        <div class="footer-grid">
          <div class="footer-brand">
            <a href="/" class="footer-logo-link" aria-label="${esc(name)}"><img class="footer-logo" src="${esc(assetUrl('logo3.png'))}" alt="${esc(name)}"></a>
            ${loc(shop.title) ? `<p>${esc(loc(shop.title))}</p>` : ''}
          </div>
          <div class="footer-col"><h2>${esc(t('footer.shop'))}</h2>${shopLinks}</div>
          ${pageLinks ? `<div class="footer-col"><h2>${esc(t('footer.info'))}</h2>${pageLinks}</div>` : ''}
          ${social ? `<div class="footer-col"><h2>${esc(t('footer.follow'))}</h2>${social}</div>` : ''}
        </div>
        <div class="footer-bottom">
          <span>© ${new Date().getFullYear()} ${esc(name)}</span>
          ${langSwitchHtml()}
        </div>
      </div>
    </footer>
  `;
  const logo = el.querySelector('.footer-logo');
  if (logo) logo.addEventListener('error', () => { logo.closest('a').remove(); }, { once: true });
  bindLangLinks(el);
}

// Trailing " — <shop name>" appended to document titles, derived from the cached
// /shop response. Empty (no suffix) until the shop loads or if it's unreachable.
function titleSuffix() {
  const shop = getCachedShop();
  const name = shop ? (loc(shop.name) || loc(shop.title)) : '';
  return name ? ` — ${name}` : '';
}

function applyI18n() {
  document.querySelectorAll('[data-i18n]').forEach(el => {
    el.textContent = t(el.dataset.i18n);
  });
  document.documentElement.lang = currentLang;
}

function setLang(lang) {
  if (lang === currentLang) return;
  currentLang = lang;
  localStorage.setItem(LANG_KEY, currentLang);
  applyI18n();
  applyShopContent();
  renderHeader();
  renderFooter();
  if (document.getElementById('home-view') && document.getElementById('home-view').style.display !== 'none') renderHome();
  if (document.getElementById('catalog-view') && document.getElementById('catalog-view').style.display !== 'none') renderCatalog();
  if (document.getElementById('product-view')) {
    const isOrderView = Boolean(new URLSearchParams(window.location.search).get('order'));
    if (isOrderView) renderOrderView();
    else renderProduct();
  }
  if (document.getElementById('page-view') && document.getElementById('page-view').style.display !== 'none') renderMarkdownPage();
  if (document.getElementById('order-status-view')) renderOrderStatus();
}

function bindLangLinks(root) {
  root.querySelectorAll('.lang-link').forEach(a => {
    a.addEventListener('click', e => {
      e.preventDefault();
      setLang(a.dataset.lang);
    });
  });
}

/* ---- Product cards ---- */

function categoryTitle(shop, id) {
  const c = ((shop && shop.categories) || []).find(x => x.id === id);
  return c ? (loc(c.title) || c.id) : '';
}

function productCard(p, shop) {
  const title = loc(p.title);
  const cat = Array.isArray(p.categories) && p.categories.length ? categoryTitle(shop, p.categories[0]) : '';
  return `
    <a class="card" href="/product?id=${encodeURIComponent(p.id)}">
      ${cardMedia(p, title)}
      <div class="card-body">
        ${cat ? `<div class="eyebrow">${esc(cat)}</div>` : ''}
        <div class="card-name">${esc(title)}</div>
        <div class="card-tagline">${esc(productTagline(p, currentLang))}</div>
        ${p.price ? `<div class="card-price">${esc(formatPrice(p.price))}</div>` : ''}
      </div>
    </a>
  `;
}

// Card media: the thumbnail plus, when the listing carries a second preview, that
// one stacked on top for the CSS hover swap (.card-image-alt).
function cardMedia(p, title) {
  const srcs = (Array.isArray(p.images) && p.images.length ? p.images : (p.image ? [p.image] : []))
    .map(src => API_BASE + src);
  return `<div class="card-media">${
    srcs[0] ? `<img class="card-image" src="${esc(srcs[0])}" alt="${esc(title)}" loading="lazy">` : ''}${
    srcs[1] ? `<img class="card-image card-image-alt" src="${esc(srcs[1])}" alt="" aria-hidden="true" loading="lazy">` : ''
  }</div>`;
}

/* ---- Home Page ---- */

async function renderHome() {
  const view = document.getElementById('home-view');
  if (!view) return;
  let products, shop, pages = [];
  try {
    [products, shop] = await Promise.all([fetchProducts(), fetchShop()]);
  } catch (e) {
    view.innerHTML = '<p class="container loading">Failed to load products.</p>';
    return;
  }
  try { pages = await fetchPages(); } catch (e) { /* story link falls back to no link */ }

  const productImages = products.filter(p => p.image).map(p => API_BASE + p.image);
  const hero = shop.hero || {};
  const heroTitle = loc(hero.title) || loc(shop.title) || loc(shop.name);
  const heroText = loc(hero.text) || loc(shop.description);
  const heroEyebrow = loc(hero.eyebrow);
  // Each slide links to its own category's catalog; no category, or one the backend
  // dropped (no products left), falls back to the whole catalog.
  const catIds = new Set((shop.categories || []).map(c => c.id));
  const heroHref = cat => cat && catIds.has(cat) ? `/catalog?category=${encodeURIComponent(cat)}` : '/catalog';
  const heroSlides = (hero.images || []).filter(i => i && i.file)
    .map(i => ({ src: assetUrl(i.file), href: heroHref(i.category) }));
  if (!heroSlides.length && productImages[0]) heroSlides.push({ src: productImages[0], href: '/catalog' });
  const story = shop.story || null;
  const storyPage = story && story.page && pages.find(p => p.id === story.page);

  const heroHtml = `
    <section class="hero">
      <div class="hero-text">
        ${heroEyebrow ? `<div class="eyebrow">${esc(heroEyebrow)}</div>` : ''}
        <h1 class="hero-heading">${esc(heroTitle)}</h1>
        ${heroText ? `<p class="hero-tagline">${esc(heroText)}</p>` : ''}
        <div class="btn-row">
          <a href="/catalog" class="btn btn-primary">${esc(t('home.cta'))}</a>
          ${storyPage ? `<a href="/${esc(storyPage.id)}" class="btn btn-outline">${esc(loc(storyPage.title))}</a>` : ''}
        </div>
      </div>
      <div class="hero-media">${heroSlidesHtml(heroSlides)}</div>
    </section>`;

  const features = (shop.features || []).filter(f => loc(f.title));
  const featuresHtml = features.length ? `
    <section class="features">
      <div class="container features-grid">
        ${features.map(f => `
          <div class="feature">
            ${icon(f.icon, 28)}
            <div><div class="feature-title">${esc(loc(f.title))}</div>${loc(f.text) ? `<div class="feature-text">${esc(loc(f.text))}</div>` : ''}</div>
          </div>`).join('')}
      </div>
    </section>` : '';

  // One product row per category (in shop order), each linking to its filtered catalog;
  // empty categories are skipped. Products with no known category land in a trailing row.
  // Cards get no shop so they drop the category eyebrow — the row title already says it.
  const productRow = (title, href, items) => `
    <section class="section container">
      <div class="section-head">
        <h2 class="section-title">${esc(title)}</h2>
        <a href="${esc(href)}" class="text-link">${esc(t('home.viewAll'))}</a>
      </div>
      <div class="grid">${items.slice(0, 8).map(p => productCard(p, null)).join('')}</div>
    </section>`;
  const cats = shop.categories || [];
  const inCat = (p, id) => Array.isArray(p.categories) && p.categories.includes(id);
  const rest = products.filter(p => !cats.some(c => inCat(p, c.id)));
  const productsHtml = cats.map(c => {
    const items = products.filter(p => inCat(p, c.id));
    return items.length ? productRow(loc(c.title) || c.id, `/catalog?category=${encodeURIComponent(c.id)}`, items) : '';
  }).join('') + (rest.length ? productRow(t(cats.length ? 'home.other' : 'home.products'), '/catalog', rest) : '');

  const storyImg = story && (story.image ? assetUrl(story.image) : (productImages[1] || productImages[0]));
  const storyHtml = story && loc(story.title) ? `
    <section class="story">
      <div class="story-media">${storyImg ? `<img src="${esc(storyImg)}" alt="" loading="lazy">` : ''}</div>
      <div class="story-text">
        ${loc(story.eyebrow) ? `<div class="eyebrow">${esc(loc(story.eyebrow))}</div>` : ''}
        <h2>${esc(loc(story.title))}</h2>
        ${loc(story.text) ? `<p>${esc(loc(story.text))}</p>` : ''}
        ${storyPage ? `<a href="/${esc(storyPage.id)}" class="text-link">${esc(t('home.story.more'))}</a>` : ''}
      </div>
    </section>` : '';

  const gallery = shop.gallery || [];
  const ig = socialLinks().find(l => l.icon === 'instagram') || socialLinks()[0];
  const handle = ig && ig.icon === 'instagram' ? '@' + ig.url.replace(/\/+$/, '').split('/').pop() : '';
  const galleryHtml = gallery.length ? `
    <section class="section container">
      <div class="section-head">
        <div>
          <h2 class="section-title">${esc(handle || t('home.gallery.title'))}</h2>
          <p class="section-sub">${esc(t('home.gallery.text'))}</p>
        </div>
        ${ig ? `<a href="${esc(ig.url)}" class="btn btn-outline" target="_blank" rel="noopener">${esc(t('home.gallery.follow'))}</a>` : ''}
      </div>
      <div class="gallery">${gallery.slice(0, 6).map(g => `<img src="${esc(assetUrl(g))}" alt="" loading="lazy">`).join('')}</div>
    </section>` : '';

  view.innerHTML = heroHtml + featuresHtml + productsHtml + storyHtml + galleryHtml;
  const heroMedia = view.querySelector('.hero-media');
  // Clicking a slide opens its catalog; the slideshow's own arrows and dots don't.
  heroMedia.addEventListener('click', e => {
    const slide = e.target.closest('img[data-href]');
    if (slide) window.location.href = slide.dataset.href;
  });
  initHeroSlider(heroMedia);
}

const HERO_INTERVAL = 6000;

function heroSlidesHtml(slides) {
  if (!slides.length) return '';
  if (slides.length === 1) return `<img src="${esc(slides[0].src)}" data-href="${esc(slides[0].href)}" alt="">`;
  return `
    <div class="hero-track">
      ${slides.map(s => `<img class="hero-slide" src="${esc(s.src)}" data-href="${esc(s.href)}" alt="">`).join('')}
    </div>
    <button type="button" class="carousel-btn carousel-prev" aria-label="${esc(t('product.prev'))}">${icon('chevronLeft', 22)}</button>
    <button type="button" class="carousel-btn carousel-next" aria-label="${esc(t('product.next'))}">${icon('chevronRight', 22)}</button>
    <div class="carousel-dots">
      ${slides.map((_, i) => `<button type="button" class="carousel-dot${i === 0 ? ' active' : ''}" data-index="${i}" aria-label="${esc(t('product.photo'))} ${i + 1}"></button>`).join('')}
    </div>`;
}

// Hero slideshow: advances every HERO_INTERVAL, also by arrows, dots and swipe. Any manual
// move restarts the timer; hovering (desktop) or a hidden tab pauses it, and reduced-motion
// users get no autoplay at all.
function initHeroSlider(media) {
  const track = media && media.querySelector('.hero-track');
  if (!track) return;
  const count = track.children.length;
  const dots = media.querySelectorAll('.carousel-dot');
  const autoplay = !window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  let current = 0;
  let timer = null;
  let hovered = false;

  function goTo(index) {
    current = (index + count) % count;
    track.style.transform = `translateX(-${current * 100}%)`;
    dots.forEach(d => d.classList.toggle('active', Number(d.dataset.index) === current));
  }
  function stop() { clearInterval(timer); timer = null; }
  function start() {
    stop();
    if (autoplay && !hovered && !document.hidden && media.isConnected) {
      timer = setInterval(() => {
        if (!media.isConnected) { stop(); return; }
        goTo(current + 1);
      }, HERO_INTERVAL);
    }
  }
  function manual(index) { goTo(index); start(); }

  media.querySelector('.carousel-prev').addEventListener('click', () => manual(current - 1));
  media.querySelector('.carousel-next').addEventListener('click', () => manual(current + 1));
  dots.forEach(d => d.addEventListener('click', () => manual(Number(d.dataset.index))));
  // pointer* with a mouse check: a tap on a phone fires a synthetic mouseenter with no
  // mouseleave, which would pause autoplay for good.
  media.addEventListener('pointerenter', e => { if (e.pointerType === 'mouse') { hovered = true; stop(); } });
  media.addEventListener('pointerleave', e => { if (e.pointerType === 'mouse') { hovered = false; start(); } });
  document.addEventListener('visibilitychange', start);

  addSwipe(media, {
    onStart() { track.style.transition = 'none'; stop(); },
    onMove(dx) { track.style.transform = `translateX(${-(current * media.offsetWidth) + dx}px)`; },
    onEnd(dx) {
      track.style.transition = '';
      goTo(Math.abs(dx) > media.offsetWidth * 0.2 ? current + (dx < 0 ? 1 : -1) : current);
      start();
    },
  });

  start();
}

// Horizontal swipe on `el`: onStart() on touch, onMove(dx) while dragging sideways, onEnd(dx)
// on release. Every onStart gets exactly one onEnd — a cancelled touch (the browser took the
// gesture, a second finger landed) or a mostly vertical drag (left to page scroll) ends with
// dx = 0, so callers can always restore transitions/timers. Pair with CSS `touch-action: pan-y`
// on `el`, otherwise the browser may claim horizontal drags itself and cancel the touch.
// `el` must not be the element being translated: once a track moves by -100%, its own box is
// off-screen and Safari stops delivering touches on the visible slide to the track's listeners.
function addSwipe(el, { onStart, onMove, onEnd }) {
  let x0 = null;
  let y0 = 0;
  let axis = null;
  const finish = dx => {
    if (x0 === null) return;
    x0 = null;
    onEnd(axis === 'x' ? dx : 0);
  };
  el.addEventListener('touchstart', e => {
    if (e.touches.length !== 1) { finish(0); return; }
    x0 = e.touches[0].clientX;
    y0 = e.touches[0].clientY;
    axis = null;
    if (onStart) onStart();
  }, { passive: true });
  el.addEventListener('touchmove', e => {
    if (x0 === null) return;
    const dx = e.touches[0].clientX - x0;
    const dy = e.touches[0].clientY - y0;
    if (!axis) {
      if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return;
      axis = Math.abs(dx) > Math.abs(dy) ? 'x' : 'y';
    }
    if (axis === 'x' && onMove) onMove(dx);
  }, { passive: true });
  el.addEventListener('touchend', e => {
    if (x0 !== null) finish(e.changedTouches[0].clientX - x0);
  }, { passive: true });
  el.addEventListener('touchcancel', () => finish(0), { passive: true });
}

/* ---- Catalog ---- */

// null = "All". Module-level so it survives re-renders (e.g. language switch).
let activeCategory = null;

// Reflect the selected category into the URL (?category=<id>) so a copied link
// reopens on the same filter and the choice survives reload. Omitted for "All" (null)
// to keep URLs clean. Preserves any other params (e.g. ?lang=); drops the query string
// entirely when nothing is left so "All" is the bare pathname, not a trailing "?".
function syncCategoryUrl() {
  const p = new URLSearchParams(window.location.search);
  if (activeCategory) p.set('category', activeCategory);
  else p.delete('category');
  const qs = p.toString();
  history.replaceState(null, '', qs ? `?${qs}` : window.location.pathname);
  syncNavActive();
}

// Re-mark the header's category links after the catalog filter changes in place.
function syncNavActive() {
  const want = activeCategory ? `/catalog?category=${encodeURIComponent(activeCategory)}` : null;
  document.querySelectorAll('#site-header .main-nav a, #site-header .mobile-nav a').forEach(a => {
    const href = a.getAttribute('href');
    if (!href.startsWith('/catalog?category=')) return;
    const on = href === want;
    a.classList.toggle('active', on);
    if (on) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
  });
}

async function renderCatalog() {
  const grid = document.getElementById('product-grid');
  if (!grid) return;
  document.title = `${t('catalog.title')}${titleSuffix()}`;
  let products, shop;
  try {
    [products, shop] = await Promise.all([fetchProducts(), fetchShop()]);
  } catch (e) {
    grid.innerHTML = '<p class="loading">Failed to load products.</p>';
    return;
  }
  const sub = document.getElementById('catalog-sub');
  if (sub) sub.textContent = loc(shop.title);
  renderCategoryBar(shop, products);
  renderProductGrid(products, shop);
}

// Categories come from /shop (localised title maps); product membership from p.categories (id list).
function renderCategoryBar(shop, products) {
  const bar = document.getElementById('category-bar');
  if (!bar) return;
  const categories = Array.isArray(shop.categories) ? shop.categories : [];
  // The URL is the source of truth on every (re)render: honour ?category= on open and
  // preserve it across language switches. An unknown/removed id falls back to "All".
  const fromUrl = new URLSearchParams(window.location.search).get('category');
  activeCategory = (fromUrl && categories.some(c => c.id === fromUrl)) ? fromUrl : null;
  syncCategoryUrl(); // normalise the URL (drop a stale/invalid ?category=)
  if (categories.length === 0) {
    bar.innerHTML = '';
    bar.style.display = 'none';
    return;
  }
  bar.style.display = '';
  const tabs = [{id: null, label: t('category.all')}].concat(
    categories.map(c => ({id: c.id, label: loc(c.title) || c.id}))
  );
  // innerHTML rewrite discards old listeners, so no clone-and-replace needed here.
  bar.innerHTML = tabs.map(tab =>
    `<button type="button" class="category-tab${tab.id === activeCategory ? ' active' : ''}" aria-pressed="${tab.id === activeCategory}" data-cat="${esc(tab.id || '')}">${esc(tab.label)}</button>`
  ).join('');
  bar.querySelectorAll('.category-tab').forEach(btn => {
    btn.addEventListener('click', () => {
      activeCategory = btn.dataset.cat || null;
      syncCategoryUrl();
      bar.querySelectorAll('.category-tab').forEach(b => {
        b.classList.toggle('active', b === btn);
        b.setAttribute('aria-pressed', String(b === btn));
      });
      renderProductGrid(products, shop);
    });
  });
}

function countLabel(n) {
  let form = 'other';
  try { form = new Intl.PluralRules(currentLang).select(n); } catch (e) { /* keep 'other' */ }
  const key = `catalog.count.${form}`;
  const tpl = t(key) === key ? t('catalog.count.other') : t(key);
  return tpl.replace('{n}', n);
}

function renderProductGrid(products, shop) {
  const grid = document.getElementById('product-grid');
  if (!grid) return;
  const filtered = activeCategory
    ? products.filter(p => Array.isArray(p.categories) && p.categories.includes(activeCategory))
    : products;
  const count = document.getElementById('catalog-count');
  if (count) count.textContent = countLabel(filtered.length);
  grid.innerHTML = filtered.length
    ? filtered.map(p => productCard(p, shop)).join('')
    : `<p class="loading">${esc(t('catalog.empty'))}</p>`;
}

/* ---- Product Page ---- */

function getProductId() {
  return new URLSearchParams(window.location.search).get('id');
}

// Fullscreen gallery over `images` starting at `index`. Arrows / keys / swipe step through
// the set; onChange(index) lets the page carousel follow along.
function openLightbox(images, index, alt, onChange) {
  const multi = images.length > 1;
  let current = index;
  const lb = document.createElement('div');
  lb.className = 'lightbox';
  lb.innerHTML = `
    <button type="button" class="lightbox-close" aria-label="Close">&#215;</button>
    <div class="lightbox-media"></div>
    ${multi ? `
      <button type="button" class="lightbox-btn lightbox-prev" aria-label="${esc(t('product.prev'))}">${icon('chevronLeft', 28)}</button>
      <button type="button" class="lightbox-btn lightbox-next" aria-label="${esc(t('product.next'))}">${icon('chevronRight', 28)}</button>
      <div class="lightbox-counter"></div>
    ` : ''}`;
  const mediaEl = lb.querySelector('.lightbox-media');
  const counter = lb.querySelector('.lightbox-counter');

  function show(i) {
    current = (i + images.length) % images.length;
    const img = images[current];
    mediaEl.innerHTML = img.type === 'video'
      ? `<video src="${esc(img.full)}" autoplay loop muted playsinline></video>`
      : `<img src="${esc(img.full)}" alt="${esc(alt)}">`;
    if (counter) counter.textContent = `${current + 1} / ${images.length}`;
    if (onChange && current !== index) { index = current; onChange(current); }
  }

  const close = () => { lb.remove(); document.body.style.overflow = ''; document.removeEventListener('keydown', onKey); };
  const onKey = e => {
    if (e.key === 'Escape') close();
    else if (multi && e.key === 'ArrowLeft') show(current - 1);
    else if (multi && e.key === 'ArrowRight') show(current + 1);
  };

  let swiped = false;
  lb.addEventListener('click', e => {
    if (swiped) { swiped = false; return; }
    if (e.target.closest('.lightbox-prev')) show(current - 1);
    else if (e.target.closest('.lightbox-next')) show(current + 1);
    else close();
  });
  if (multi) {
    addSwipe(lb, {
      onStart() { swiped = false; },
      onEnd(dx) { if (Math.abs(dx) > 50) { swiped = true; show(current + (dx < 0 ? 1 : -1)); } },
    });
  }

  show(current);
  document.body.appendChild(lb);
  document.body.style.overflow = 'hidden';
  document.addEventListener('keydown', onKey);
}

function renderCarousel(container, images, altText) {
  const multi = images.length > 1;

  const slideHtml = img => img.type === 'video'
    ? `<video class="carousel-slide" src="${esc(img.preview)}" loop muted playsinline preload="auto"></video>`
    : `<img class="carousel-slide" src="${esc(img.preview)}" alt="${esc(altText)}">`;

  const thumbHtml = (img, i) => `
    <button type="button" class="carousel-thumb${i === 0 ? ' active' : ''}" data-index="${i}" aria-label="${esc(t('product.photo'))} ${i + 1}">
      ${img.type === 'video'
        ? `<video src="${esc(img.preview)}" muted playsinline preload="metadata"></video>`
        : `<img src="${esc(img.preview)}" alt="" loading="lazy">`}
    </button>`;

  container.innerHTML = `
    <div class="carousel-viewport">
      <div class="carousel-track">
        ${images.map(slideHtml).join('')}
      </div>
      ${multi ? `
        <button type="button" class="carousel-btn carousel-prev" aria-label="${esc(t('product.prev'))}">${icon('chevronLeft', 22)}</button>
        <button type="button" class="carousel-btn carousel-next" aria-label="${esc(t('product.next'))}">${icon('chevronRight', 22)}</button>
        <div class="carousel-dots">
          ${images.map((_, i) => `<button type="button" class="carousel-dot${i === 0 ? ' active' : ''}" data-index="${i}" aria-label="${esc(t('product.photo'))} ${i + 1}"></button>`).join('')}
        </div>
      ` : ''}
    </div>
    ${multi ? `<div class="carousel-thumbs">${images.map(thumbHtml).join('')}</div>` : ''}
  `;

  const slides = container.querySelectorAll('.carousel-slide');
  slides.forEach((slide, i) => {
    slide.style.cursor = 'zoom-in';
    slide.addEventListener('click', () => openLightbox(images, i, altText, container._goTo));
  });

  // Mobile Chrome only autoplays a muted video while it is actually visible; an off-screen
  // carousel slide (videos always render after photos) never starts on its own. Drive playback
  // of the active slide's video explicitly and pause the rest.
  function syncVideoPlayback(index) {
    slides.forEach((slide, i) => {
      if (slide.tagName !== 'VIDEO') return;
      if (i === index) slide.play().catch(() => {});
      else slide.pause();
    });
  }
  syncVideoPlayback(0);

  if (!multi) { container._goTo = () => {}; return; }

  let current = 0;
  const viewport = container.querySelector('.carousel-viewport');
  const track = container.querySelector('.carousel-track');
  const pickers = container.querySelectorAll('.carousel-dot, .carousel-thumb');

  function goTo(index) {
    current = (index + images.length) % images.length;
    track.style.transform = `translateX(-${current * 100}%)`;
    pickers.forEach(d => d.classList.toggle('active', Number(d.dataset.index) === current));
    syncVideoPlayback(current);
    syncCarouselUrl(current);
    if (container._onSlide) container._onSlide(current);
  }

  container.querySelector('.carousel-prev').addEventListener('click', () => goTo(current - 1));
  container.querySelector('.carousel-next').addEventListener('click', () => goTo(current + 1));
  pickers.forEach(d => d.addEventListener('click', () => goTo(Number(d.dataset.index))));

  container._goTo = goTo;

  addSwipe(viewport, {
    onStart() { track.style.transition = 'none'; },
    onMove(dx) { track.style.transform = `translateX(${-(current * viewport.offsetWidth) + dx}px)`; },
    onEnd(dx) {
      track.style.transition = '';
      if (Math.abs(dx) > viewport.offsetWidth * 0.25) goTo(current + (dx < 0 ? 1 : -1));
      else track.style.transform = `translateX(-${current * 100}%)`;
    },
  });
}

// Swatch colors for one attribute: {valueKey: '#rrggbb'} from product.attr_colors, or null
// unless every listed value has a valid color (a partial set falls back to text pills).
function swatchColors(product, attrKey, valKeys) {
  const map = product.attr_colors && product.attr_colors[attrKey];
  if (!map) return null;
  const ok = valKeys.length > 0 && valKeys.every(v => /^#[0-9a-fA-F]{6}$/.test(map[v] || ''));
  return ok ? map : null;
}

function renderVariants(product, carouselEl) {
  const container = document.getElementById('product-variants');
  if (!container) return;

  const attrs = product.attrs || {};
  const attrKeys = Object.keys(attrs);
  const attrImages = product.attr_images || {};
  const attrValuesOrder = product.attr_values_order || {};
  const rawImages = product.images || [];
  if (attrKeys.length === 0) { container.innerHTML = ''; return; }

  const params = new URLSearchParams(window.location.search);
  const selections = {};
  attrKeys.forEach(key => {
    const langData = attrs[key];
    if (!langData) return;
    const valKeys = attrValuesOrder[key] || Object.keys(langData.values);
    const fromUrl = params.get(key);
    selections[key] = valKeys.includes(fromUrl) ? fromUrl : valKeys[0];
  });

  function syncUrl() {
    const p = new URLSearchParams(window.location.search);
    Object.entries(selections).forEach(([k, v]) => p.set(k, v));
    history.replaceState(null, '', `?${p.toString()}`);
  }

  function getPrice() {
    const base = selectPrice(product, currentLang);
    let total = base.value;
    const attrPrices = product.attr_prices || {};
    attrKeys.forEach(key => {
      total += (attrPrices[key] && attrPrices[key][selections[key]]) || 0;
    });
    return formatPrice({ currency: base.currency, value: total });
  }

  function updatePrice() {
    const el = document.getElementById('spec-price');
    if (el) el.textContent = getPrice();
  }

  function render() {
    container.innerHTML = attrKeys.map(key => {
      const langData = attrs[key];
      if (!langData) return '';
      const descHtml = langData.description ? `<div class="variant-desc">${marked.parse(langData.description)}</div>` : '';
      const valKeys = attrValuesOrder[key] || Object.keys(langData.values);
      const colors = swatchColors(product, key, valKeys);
      const selected = langData.values[selections[key]];
      const nameHtml = colors
        ? `${esc(langData.title)}: <span class="variant-selected">${esc((selected && selected.title) || '')}</span>`
        : esc(langData.title);
      return `
        <div class="variant-group">
          <div class="variant-name">${nameHtml}</div>
          <div class="variant-options">
            ${valKeys.map(valKey => {
              const val = langData.values[valKey];
              if (!val) return '';
              const active = selections[key] === valKey;
              if (colors) {
                return `
              <button type="button" class="variant-option variant-swatch${active ? ' active' : ''}" aria-pressed="${active}"
                      aria-label="${esc(val.title)}" title="${esc(val.title)}"
                      data-variant="${esc(key)}" data-option="${esc(valKey)}">
                <span class="swatch-dot" style="background:${esc(colors[valKey])}"></span>
              </button>`;
              }
              const label = val.prefix ? `${val.prefix} ${val.title}` : val.title;
              return `
              <button type="button" class="variant-option${active ? ' active' : ''}" aria-pressed="${active}"
                      data-variant="${esc(key)}" data-option="${esc(valKey)}">
                ${esc(label)}
              </button>`;
            }).join('')}
          </div>
          ${descHtml}
        </div>
      `;
    }).join('');

    container.querySelectorAll('.variant-option').forEach(btn => {
      btn.addEventListener('click', () => {
        const key = btn.dataset.variant;
        const val = btn.dataset.option;
        selections[key] = val;
        syncUrl();
        render();
        updatePrice();
        if (carouselEl && carouselEl._goTo && attrImages[key] && attrImages[key][val]) {
          const imgPath = attrImages[key][val];
          const idx = rawImages.findIndex(img => img.preview === imgPath);
          if (idx !== -1) carouselEl._goTo(idx);
        }
      });
    });
  }

  // Reverse of the variant→image jump: landing on a slide that attr_images maps to a
  // value selects that value. Never calls _goTo, so the two directions can't loop.
  if (carouselEl) {
    carouselEl._onSlide = idx => {
      const img = rawImages[idx];
      if (!img) return;
      let changed = false;
      attrKeys.forEach(key => {
        const map = attrImages[key];
        if (!map || map[selections[key]] === img.preview) return;
        const val = Object.keys(map).find(v => map[v] === img.preview);
        if (val && attrs[key] && attrs[key].values[val]) {
          selections[key] = val;
          changed = true;
        }
      });
      if (!changed) return;
      syncUrl();
      render();
      updatePrice();
    };
  }

  syncUrl();
  render();
  updatePrice();
}

async function renderProduct() {
  const id = getProductId();
  let product;
  try {
    product = await fetchProduct(id, currentLang);
  } catch (e) {
    window.location.href = '/';
    return;
  }

  syncProductLangUrl();

  document.title = `${product.name || ''}${titleSuffix()}`;

  const carouselEl = document.getElementById('product-carousel');
  if (carouselEl) {
    const images = (product.images || []).map(img => ({
      type: img.type || 'image',
      preview: API_BASE + img.preview,
      full: API_BASE + img.full,
    }));
    renderCarousel(carouselEl, images, product.name || '');
  }

  document.getElementById('product-name').textContent = product.name || '';

  const descEl = document.getElementById('product-description');
  if (descEl) {
    const md = product.description || '';
    descEl.innerHTML = md ? marked.parse(md) : '';
  }

  // Replace order button to avoid stacking listeners across re-renders
  const orderBtn = document.querySelector('#product-view .order-btn');
  if (orderBtn) {
    if (product.available === false) {
      orderBtn.disabled = true;
      orderBtn.textContent = t('product.unavailable');
    } else {
      const newBtn = orderBtn.cloneNode(true);
      orderBtn.parentNode.replaceChild(newBtn, orderBtn);
      newBtn.addEventListener('click', () => {
        const p = new URLSearchParams(window.location.search);
        p.set('order', '1');
        window.location.search = p.toString();
      });
    }
  }

  const priceEl = document.getElementById('spec-price');
  if (priceEl) priceEl.textContent = formatPrice(selectPrice(product, currentLang));

  const specsEl = document.getElementById('product-specs');
  if (specsEl) {
    const specs = Object.values(product.specs || {});
    specsEl.innerHTML = specs.map(s => `
      <div class="spec-row">
        <span class="spec-label">${esc(s.title)}</span>
        <span class="spec-value">${esc(s.value)}</span>
      </div>
    `).join('');
  }

  renderVariants(product, carouselEl || null);
  renderProductContext(product);

  // Restore the carousel slide from the URL (URL wins on load). Variant init never
  // moves the carousel — only variant clicks do — so the URL is the sole authority here.
  // A slide mapped in attr_images also selects its variant (_onSlide).
  const imgIdx = parseInt(new URLSearchParams(window.location.search).get('img'), 10);
  if (carouselEl && carouselEl._goTo && imgIdx > 0 && imgIdx < (product.images || []).length) {
    carouselEl._goTo(imgIdx);
  }
}

// Breadcrumbs, category eyebrow, short tagline and the "you may also like" row.
// All come from the /products listing and /shop; failures just leave them empty.
async function renderProductContext(product) {
  let products = [], shop = {};
  try {
    [products, shop] = await Promise.all([fetchProducts(), fetchShop()]);
  } catch (e) { /* context is decorative */ }
  const item = products.find(p => p.id === product.id);
  const catId = (product.categories && product.categories[0]) || (item && item.categories && item.categories[0]);
  const catTitle = catId ? categoryTitle(shop, catId) : '';

  const crumbs = document.getElementById('product-breadcrumbs');
  if (crumbs) {
    crumbs.innerHTML = `<a href="/">${esc(t('nav.home'))}</a><span aria-hidden="true">/</span>` +
      (catTitle
        ? `<a href="/catalog?category=${encodeURIComponent(catId)}">${esc(catTitle)}</a>`
        : `<a href="/catalog">${esc(t('nav.products'))}</a>`) +
      `<span aria-hidden="true">/</span><span aria-current="page">${esc(product.name || '')}</span>`;
  }
  const eyebrow = document.getElementById('product-category');
  if (eyebrow) eyebrow.textContent = catTitle;
  const tagline = document.getElementById('product-tagline');
  if (tagline) tagline.textContent = item ? productTagline(item, currentLang) : '';

  const related = document.getElementById('related-products');
  const section = document.getElementById('related-section');
  if (!related || !section) return;
  const others = products.filter(p => p.id !== product.id);
  const sameCat = others.filter(p => catId && Array.isArray(p.categories) && p.categories.includes(catId));
  const picks = sameCat.concat(others.filter(p => !sameCat.includes(p))).slice(0, 3);
  section.hidden = picks.length === 0;
  related.innerHTML = picks.map(p => productCard(p, shop)).join('');
}

/* ---- Order View ---- */

async function renderOrderView() {
  const id = getProductId();
  let product;
  try {
    product = await fetchProduct(id, currentLang);
  } catch (e) {
    window.location.href = '/';
    return;
  }

  if (product.available === false) {
    const p = new URLSearchParams(window.location.search);
    p.delete('order');
    window.location.search = p.toString();
    return;
  }

  syncProductLangUrl();

  let shop;
  try { shop = await fetchShop(); } catch (e) { shop = {}; }
  const countries = (shop && shop.countries) || {};

  document.title = `${product.name || ''}${titleSuffix()}`;

  // Clone-and-replace the form so the submit handler captures fresh closure state
  // (currentLang, product, etc.) on every render. Must happen before autocomplete
  // setup so those sections look up their inputs inside the new form.
  const formOrig = document.getElementById('order-form');
  if (formOrig) {
    const formFresh = formOrig.cloneNode(true);
    formOrig.parentNode.replaceChild(formFresh, formOrig);
  }

  // Promo code state. The discount is previewed here and re-validated by POST /orders,
  // which applies the same formula (product.Discount) to the same subtotal in cents.
  let summarySubtotalCents = 0;
  let summaryCurrency = '';
  let appliedPromo = null; // {code, discount_percent} from GET /products/{id}/promo-codes/{code}

  // The discounted total is rounded to whole currency units (2124.15 → 2124).
  function promoTotalCents() {
    if (!appliedPromo) return summarySubtotalCents;
    return Math.round(summarySubtotalCents * (100 - appliedPromo.discount_percent) / 10000) * 100;
  }

  // The subtotal can still carry cents (attr addons); show them exactly so the form
  // matches the invoice.
  function formatCents(cents) {
    const s = formatPrice({ currency: summaryCurrency, value: cents / 100 });
    if (cents % 100 === 0) return s;
    const whole = formatPrice({ currency: summaryCurrency, value: Math.trunc(cents / 100) });
    return whole.replace(/\d(?!.*\d)/, m => `${m}.${String(cents % 100).padStart(2, '0')}`);
  }

  function renderSummaryPrice() {
    const el = document.querySelector('#order-summary .order-summary-price');
    if (!el) return;
    if (!appliedPromo) {
      el.textContent = formatPrice({ currency: summaryCurrency, value: summarySubtotalCents / 100 });
      return;
    }
    el.innerHTML = `<s class="order-summary-price-old">${esc(formatCents(summarySubtotalCents))}</s> `
      + `${esc(formatCents(promoTotalCents()))}`
      + `<div class="order-summary-promo">${esc(appliedPromo.code)}: ${esc(String(appliedPromo.discount_percent))}%</div>`;
  }

  // Build product summary
  const summaryEl = document.getElementById('order-summary');
  if (summaryEl) {
    const attrs = product.attrs || {};
    const attrKeys = Object.keys(attrs);
    const attrValuesOrder = product.attr_values_order || {};
    const params = new URLSearchParams(window.location.search);
    const selections = {};
    attrKeys.forEach(key => {
      const langData = attrs[key];
      if (!langData) return;
      const valKeys = attrValuesOrder[key] || Object.keys(langData.values);
      const fromUrl = params.get(key);
      selections[key] = valKeys.includes(fromUrl) ? fromUrl : valKeys[0];
    });

    const base = selectPrice(product, currentLang);
    let total = base.value;
    const attrPrices = product.attr_prices || {};
    attrKeys.forEach(key => {
      total += (attrPrices[key] && attrPrices[key][selections[key]]) || 0;
    });
    const priceStr = formatPrice({ currency: base.currency, value: total });
    summarySubtotalCents = Math.round(total * 100);
    summaryCurrency = base.currency;

    const attrsHtml = attrKeys.map(key => {
      const langData = attrs[key];
      if (!langData) return '';
      const valKey = selections[key];
      const val = langData.values[valKey];
      const valTitle = (val && val.title) || valKey;
      const colors = swatchColors(product, key, attrValuesOrder[key] || Object.keys(langData.values));
      const valHtml = colors
        ? `<span class="swatch-dot swatch-dot-sm" style="background:${esc(colors[valKey])}"></span>${esc(valTitle)}`
        : esc((val && val.prefix) ? `${val.prefix} ${valTitle}` : valTitle);
      const descHtml = langData.description ? `<div class="order-summary-attr-desc">${marked.parse(langData.description)}</div>` : '';
      return `<div class="order-summary-attr"><span class="order-summary-attr-label">${esc(langData.title)}:</span> ${valHtml}${descHtml}</div>`;
    }).join('');

    // Resolve image: prefer attr_images match for current selections, fall back to first image
    const attrImages = product.attr_images || {};
    const rawImages = product.images || [];
    let resolvedImage = rawImages[0] ? API_BASE + rawImages[0].preview : '';
    for (const key of attrKeys) {
      const val = selections[key];
      if (attrImages[key] && attrImages[key][val]) {
        resolvedImage = API_BASE + attrImages[key][val];
        break;
      }
    }

    summaryEl.innerHTML = `
      ${resolvedImage ? `<img class="order-summary-image" src="${esc(resolvedImage)}" alt="${esc(product.name || '')}">` : ''}
      <h2 class="order-summary-name">${esc(product.name || '')}</h2>
      ${attrsHtml}
      <div class="order-summary-price">${esc(priceStr)}</div>
    `;
  }

  // Promo code field
  const promoInput = document.getElementById('order-promo');
  const promoApply = document.getElementById('order-promo-apply');
  const promoMsg = document.getElementById('order-promo-msg');

  function showPromoMsg(key) {
    if (!promoMsg) return;
    promoMsg.textContent = key ? t(key) : '';
    promoMsg.style.display = key ? '' : 'none';
    promoMsg.classList.remove('promo-msg-ok');
  }

  // The summary with the discounted price sits above the fold on phones, so confirm
  // the discount next to the field too.
  function showPromoApplied() {
    if (!promoMsg) return;
    promoMsg.textContent = t('order.promo.applied')
      .replace('{percent}', String(appliedPromo.discount_percent))
      .replace('{total}', formatCents(promoTotalCents()));
    promoMsg.style.display = '';
    promoMsg.classList.add('promo-msg-ok');
  }

  async function applyPromo() {
    const code = (promoInput && promoInput.value || '').trim();
    appliedPromo = null;
    showPromoMsg('');
    if (!code) { renderSummaryPrice(); return; }
    try {
      const res = await fetchPromoCode(getProductId(), code);
      if (res) { appliedPromo = res; showPromoApplied(); }
      else showPromoMsg('order.promo.invalid');
    } catch (_) {
      showPromoMsg('order.promo.error');
    }
    renderSummaryPrice();
  }

  if (promoInput) {
    promoInput.addEventListener('keydown', e => {
      if (e.key === 'Enter') { e.preventDefault(); applyPromo(); }
    });
    promoInput.addEventListener('input', () => {
      if (appliedPromo) { appliedPromo = null; renderSummaryPrice(); }
      showPromoMsg('');
    });
    // The form is re-cloned on every render (e.g. language switch), which keeps the typed value.
    if (promoInput.value.trim()) applyPromo();
  }
  if (promoApply) promoApply.addEventListener('click', applyPromo);

  // Back link: remove order=1, reload to product view
  const backOrig = document.getElementById('order-back');
  if (backOrig) {
    const backBtn = backOrig.cloneNode(true);
    backOrig.parentNode.replaceChild(backBtn, backOrig);
    backBtn.addEventListener('click', e => {
      e.preventDefault();
      const p = new URLSearchParams(window.location.search);
      p.delete('order');
      window.location.search = p.toString();
    });
  }

  // Country select (populated from /shop) → phone code sync
  const countryOrig = document.getElementById('order-country');
  const phoneCodeEl = document.getElementById('order-phone-code');
  const phoneNumberInput = document.getElementById('order-phone-number');
  let countrySelect = countryOrig;
  if (countryOrig) {
    const fresh = countryOrig.cloneNode(false);
    countryOrig.parentNode.replaceChild(fresh, countryOrig);
    countrySelect = fresh;
    const codes = Object.keys(countries);
    countrySelect.innerHTML = codes.map(code => {
      const c = countries[code] || {};
      const cName = (c.name && (c.name[currentLang] || Object.values(c.name)[0])) || code;
      const flag = c.flag || '';
      const label = (flag ? flag + ' ' : '') + cName;
      return `<option value="${esc(code)}">${esc(label)}</option>`;
    }).join('');
  }
  function syncPhoneCode() {
    const code = countrySelect && countrySelect.value;
    const info = countries[code] || {};
    if (phoneCodeEl) phoneCodeEl.textContent = info.phone_code || '';
  }
  if (countrySelect) countrySelect.addEventListener('change', syncPhoneCode);
  syncPhoneCode();

  // Strip any non-digit characters as the user types (cap at E.164 max of 15)
  if (phoneNumberInput) {
    phoneNumberInput.addEventListener('input', () => {
      const pos = phoneNumberInput.selectionStart;
      const original = phoneNumberInput.value;
      const cleaned = original.replace(/\D/g, '').replace(/^0+/, '').slice(0, 15);
      if (cleaned !== original) {
        phoneNumberInput.value = cleaned;
        phoneNumberInput.setSelectionRange(pos - 1, pos - 1);
      }
    });
  }

  // Delivery method toggle: Branch/Postomat ↔ Courier.
  // Re-query inputs each call: the novapost and address inputs are clone-and-replaced
  // by their autocomplete blocks below, so cached references go stale.
  function applyDeliveryToggle(value) {
    const isCourier = value === 'courier';
    const novapostField = document.getElementById('order-novapost-field');
    const addressField = document.getElementById('order-address-field');
    const middlenameField = document.getElementById('order-middlename-field');
    const novapostInput = document.getElementById('order-novapost');
    const addressInput = document.getElementById('order-address');
    const buildingInput = document.getElementById('order-building');
    const middlenameInput = document.getElementById('order-middlename');
    if (novapostField) novapostField.style.display = isCourier ? 'none' : '';
    if (addressField) addressField.style.display = isCourier ? '' : 'none';
    if (middlenameField) middlenameField.style.display = isCourier ? '' : 'none';
    if (novapostInput) novapostInput.required = !isCourier;
    if (addressInput) addressInput.required = isCourier;
    if (buildingInput) buildingInput.required = isCourier;
    if (middlenameInput) middlenameInput.required = isCourier;
  }
  document.querySelectorAll('input[name="delivery"]').forEach(radio => {
    radio.addEventListener('change', e => applyDeliveryToggle(e.target.value));
  });
  const checkedDelivery = document.querySelector('input[name="delivery"]:checked');
  if (checkedDelivery) applyDeliveryToggle(checkedDelivery.value);

  // City autocomplete
  const cityOrig = document.getElementById('order-city');
  if (cityOrig) {
    // Clone to remove stacked listeners from prior renders (language switches)
    const cityInput = cityOrig.cloneNode(true);
    cityOrig.parentNode.replaceChild(cityInput, cityOrig);

    // Remove any leftover dropdown from a prior render
    const existingDd = document.getElementById('city-autocomplete-dropdown');
    if (existingDd) existingDd.remove();
    cityInput.parentNode.classList.remove('open');

    const cityRefInput = document.getElementById('order-city-ref');
    let debounceTimer = null;
    let suggestions = [];
    let activeIdx = -1;

    function getDropdown() {
      let dd = document.getElementById('city-autocomplete-dropdown');
      if (!dd) {
        dd = document.createElement('ul');
        dd.id = 'city-autocomplete-dropdown';
        dd.className = 'autocomplete-dropdown';
        cityInput.parentNode.appendChild(dd);
        cityInput.parentNode.classList.add('open');
      }
      return dd;
    }

    function closeDropdown() {
      const dd = document.getElementById('city-autocomplete-dropdown');
      if (dd) dd.remove();
      cityInput.parentNode.classList.remove('open');
      suggestions = [];
      activeIdx = -1;
    }

    function renderDropdown(items) {
      suggestions = items;
      activeIdx = -1;
      if (items.length === 0) { closeDropdown(); return; }
      const dd = getDropdown();
      dd.innerHTML = items.map((item, i) =>
        `<li class="autocomplete-item" data-index="${i}">${esc(item.name)}</li>`
      ).join('');
      dd.querySelectorAll('.autocomplete-item').forEach(li => {
        li.addEventListener('mousedown', e => {
          e.preventDefault(); // prevent input blur before selection
          selectCity(suggestions[Number(li.dataset.index)]);
        });
      });
    }

    function selectCity(item) {
      cityInput.value = item.name;
      if (cityRefInput) cityRefInput.value = item.ref;
      closeDropdown();
      // Enable branch and address fields now that a city is selected
      const novapostInput = document.getElementById('order-novapost');
      const novapostRefInput = document.getElementById('order-novapost-ref');
      if (novapostInput) { novapostInput.disabled = false; novapostInput.value = ''; }
      if (novapostRefInput) novapostRefInput.value = '';
      const addressInput = document.getElementById('order-address');
      const addressRefInput = document.getElementById('order-address-ref');
      if (addressInput) { addressInput.disabled = false; addressInput.value = ''; }
      if (addressRefInput) addressRefInput.value = '';
      const buildingInput = document.getElementById('order-building');
      const aptInput = document.getElementById('order-apt');
      if (buildingInput) { buildingInput.disabled = false; buildingInput.value = ''; }
      if (aptInput) { aptInput.disabled = false; aptInput.value = ''; }
    }

    function setActive(idx) {
      activeIdx = idx;
      const dd = document.getElementById('city-autocomplete-dropdown');
      if (!dd) return;
      dd.querySelectorAll('.autocomplete-item').forEach((li, i) => {
        li.classList.toggle('active', i === activeIdx);
        if (i === activeIdx) li.scrollIntoView({ block: 'nearest' });
      });
    }

    cityInput.addEventListener('input', () => {
      if (cityRefInput) cityRefInput.value = '';
      // City changed — disable branch, address, and courier fields; clear their values
      const novapostInput = document.getElementById('order-novapost');
      const novapostRefInput = document.getElementById('order-novapost-ref');
      if (novapostInput) { novapostInput.disabled = true; novapostInput.value = ''; }
      if (novapostRefInput) novapostRefInput.value = '';
      const addressInput = document.getElementById('order-address');
      const addressRefInput = document.getElementById('order-address-ref');
      if (addressInput) { addressInput.disabled = true; addressInput.value = ''; }
      if (addressRefInput) addressRefInput.value = '';
      const buildingInput = document.getElementById('order-building');
      const aptInput = document.getElementById('order-apt');
      if (buildingInput) { buildingInput.disabled = true; buildingInput.value = ''; }
      if (aptInput) { aptInput.disabled = true; aptInput.value = ''; }
      const q = cityInput.value.trim();
      clearTimeout(debounceTimer);
      if (q.length < 2) { closeDropdown(); return; }
      debounceTimer = setTimeout(async () => {
        try {
          const items = await fetchNovaPoshtaCities(q);
          renderDropdown(items || []);
        } catch (_) {
          closeDropdown();
        }
      }, 300);
    });

    cityInput.addEventListener('keydown', e => {
      if (!document.getElementById('city-autocomplete-dropdown')) return;
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setActive(Math.min(activeIdx + 1, suggestions.length - 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setActive(Math.max(activeIdx - 1, 0));
      } else if (e.key === 'Enter' && activeIdx >= 0) {
        e.preventDefault();
        selectCity(suggestions[activeIdx]);
      } else if (e.key === 'Escape') {
        closeDropdown();
      }
    });

    cityInput.addEventListener('blur', () => {
      setTimeout(() => {
        closeDropdown();
        // If the user typed manually without selecting from autocomplete, clear the field
        if (cityRefInput && !cityRefInput.value && cityInput.value.trim()) {
          cityInput.value = '';
        }
      }, 150);
    });
  }

  // Branch autocomplete
  const novapostOrig = document.getElementById('order-novapost');
  if (novapostOrig) {
    // Clone to remove stacked listeners from prior renders (language switches)
    const novapostInput = novapostOrig.cloneNode(true);
    novapostOrig.parentNode.replaceChild(novapostInput, novapostOrig);

    // Remove any leftover dropdown from a prior render
    const existingBranchDd = document.getElementById('branch-autocomplete-dropdown');
    if (existingBranchDd) existingBranchDd.remove();
    novapostInput.parentNode.classList.remove('open');

    const novapostRefInput = document.getElementById('order-novapost-ref');
    let branchDebounceTimer = null;
    let branchSuggestions = [];
    let branchActiveIdx = -1;

    function getBranchDropdown() {
      let dd = document.getElementById('branch-autocomplete-dropdown');
      if (!dd) {
        dd = document.createElement('ul');
        dd.id = 'branch-autocomplete-dropdown';
        dd.className = 'autocomplete-dropdown';
        novapostInput.parentNode.appendChild(dd);
        novapostInput.parentNode.classList.add('open');
      }
      return dd;
    }

    function closeBranchDropdown() {
      const dd = document.getElementById('branch-autocomplete-dropdown');
      if (dd) dd.remove();
      novapostInput.parentNode.classList.remove('open');
      branchSuggestions = [];
      branchActiveIdx = -1;
    }

    function renderBranchDropdown(items) {
      branchSuggestions = items;
      branchActiveIdx = -1;
      if (items.length === 0) { closeBranchDropdown(); return; }
      const dd = getBranchDropdown();
      dd.innerHTML = items.map((item, i) =>
        `<li class="autocomplete-item" data-index="${i}">${esc(item.name)}</li>`
      ).join('');
      dd.querySelectorAll('.autocomplete-item').forEach(li => {
        li.addEventListener('mousedown', e => {
          e.preventDefault();
          selectBranch(branchSuggestions[Number(li.dataset.index)]);
        });
      });
    }

    function selectBranch(item) {
      novapostInput.value = item.name;
      if (novapostRefInput) novapostRefInput.value = item.ref;
      closeBranchDropdown();
    }

    function setBranchActive(idx) {
      branchActiveIdx = idx;
      const dd = document.getElementById('branch-autocomplete-dropdown');
      if (!dd) return;
      dd.querySelectorAll('.autocomplete-item').forEach((li, i) => {
        li.classList.toggle('active', i === branchActiveIdx);
        if (i === branchActiveIdx) li.scrollIntoView({ block: 'nearest' });
      });
    }

    novapostInput.addEventListener('input', () => {
      if (novapostRefInput) novapostRefInput.value = '';
      const cityRef = document.getElementById('order-city-ref').value;
      const q = novapostInput.value.trim();
      clearTimeout(branchDebounceTimer);
      if (!cityRef || q.length < 2) { closeBranchDropdown(); return; }
      branchDebounceTimer = setTimeout(async () => {
        try {
          const items = await fetchNovaPoshtaBranches(cityRef, q);
          renderBranchDropdown(items || []);
        } catch (_) {
          closeBranchDropdown();
        }
      }, 300);
    });

    novapostInput.addEventListener('keydown', e => {
      if (!document.getElementById('branch-autocomplete-dropdown')) return;
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setBranchActive(Math.min(branchActiveIdx + 1, branchSuggestions.length - 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setBranchActive(Math.max(branchActiveIdx - 1, 0));
      } else if (e.key === 'Enter' && branchActiveIdx >= 0) {
        e.preventDefault();
        selectBranch(branchSuggestions[branchActiveIdx]);
      } else if (e.key === 'Escape') {
        closeBranchDropdown();
      }
    });

    novapostInput.addEventListener('blur', () => {
      setTimeout(() => {
        closeBranchDropdown();
        // If the user typed manually without selecting from autocomplete, clear the field
        if (novapostRefInput && !novapostRefInput.value && novapostInput.value.trim()) {
          novapostInput.value = '';
        }
      }, 150);
    });
  }

  // Street autocomplete
  const addressOrig = document.getElementById('order-address');
  if (addressOrig) {
    // Clone to remove stacked listeners from prior renders (language switches)
    const addressInput = addressOrig.cloneNode(true);
    addressOrig.parentNode.replaceChild(addressInput, addressOrig);

    // Remove any leftover dropdown from a prior render
    const existingStreetDd = document.getElementById('street-autocomplete-dropdown');
    if (existingStreetDd) existingStreetDd.remove();
    addressInput.parentNode.classList.remove('open');

    const addressRefInput = document.getElementById('order-address-ref');
    let streetDebounceTimer = null;
    let streetSuggestions = [];
    let streetActiveIdx = -1;

    function getStreetDropdown() {
      let dd = document.getElementById('street-autocomplete-dropdown');
      if (!dd) {
        dd = document.createElement('ul');
        dd.id = 'street-autocomplete-dropdown';
        dd.className = 'autocomplete-dropdown';
        addressInput.parentNode.appendChild(dd);
        addressInput.parentNode.classList.add('open');
      }
      return dd;
    }

    function closeStreetDropdown() {
      const dd = document.getElementById('street-autocomplete-dropdown');
      if (dd) dd.remove();
      addressInput.parentNode.classList.remove('open');
      streetSuggestions = [];
      streetActiveIdx = -1;
    }

    function renderStreetDropdown(items) {
      streetSuggestions = items;
      streetActiveIdx = -1;
      if (items.length === 0) { closeStreetDropdown(); return; }
      const dd = getStreetDropdown();
      dd.innerHTML = items.map((item, i) =>
        `<li class="autocomplete-item" data-index="${i}">${esc(item.name)}</li>`
      ).join('');
      dd.querySelectorAll('.autocomplete-item').forEach(li => {
        li.addEventListener('mousedown', e => {
          e.preventDefault();
          selectStreet(streetSuggestions[Number(li.dataset.index)]);
        });
      });
    }

    function selectStreet(item) {
      addressInput.value = item.name;
      if (addressRefInput) addressRefInput.value = item.ref;
      closeStreetDropdown();
    }

    function setStreetActive(idx) {
      streetActiveIdx = idx;
      const dd = document.getElementById('street-autocomplete-dropdown');
      if (!dd) return;
      dd.querySelectorAll('.autocomplete-item').forEach((li, i) => {
        li.classList.toggle('active', i === streetActiveIdx);
        if (i === streetActiveIdx) li.scrollIntoView({ block: 'nearest' });
      });
    }

    addressInput.addEventListener('input', () => {
      if (addressRefInput) addressRefInput.value = '';
      const cityRef = document.getElementById('order-city-ref').value;
      const q = addressInput.value.trim();
      clearTimeout(streetDebounceTimer);
      if (!cityRef || q.length < 2) { closeStreetDropdown(); return; }
      streetDebounceTimer = setTimeout(async () => {
        try {
          const items = await fetchNovaPoshtaStreets(cityRef, q);
          renderStreetDropdown(items || []);
        } catch (_) {
          closeStreetDropdown();
        }
      }, 300);
    });

    addressInput.addEventListener('keydown', e => {
      if (!document.getElementById('street-autocomplete-dropdown')) return;
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setStreetActive(Math.min(streetActiveIdx + 1, streetSuggestions.length - 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setStreetActive(Math.max(streetActiveIdx - 1, 0));
      } else if (e.key === 'Enter' && streetActiveIdx >= 0) {
        e.preventDefault();
        selectStreet(streetSuggestions[streetActiveIdx]);
      } else if (e.key === 'Escape') {
        closeStreetDropdown();
      }
    });

    addressInput.addEventListener('blur', () => {
      setTimeout(() => {
        closeStreetDropdown();
        // If the user typed manually without selecting from autocomplete, clear the field
        if (addressRefInput && !addressRefInput.value && addressInput.value.trim()) {
          addressInput.value = '';
        }
      }, 150);
    });
  }

  // Form submit — POST /orders, redirect to payment_url on success
  const form = document.getElementById('order-form');
  const errorEl = document.getElementById('order-error');
  const submitBtn = form ? form.querySelector('button[type="submit"]') : null;
  const consentEl = document.getElementById('order-consent');
  const consentNoticeEl = document.getElementById('order-consent-notice');
  if (consentNoticeEl) consentNoticeEl.innerHTML = t('order.consent.notice');
  const consentLabelEl = document.getElementById('order-consent-label');
  if (consentLabelEl) {
    const offerLink = `<a href="/offer" target="_blank" rel="noopener">${t('order.consent.offer')}</a>`;
    const policyLink = `<a href="/privacy" target="_blank" rel="noopener">${t('order.consent.policy')}</a>`;
    consentLabelEl.innerHTML = t('order.consent')
      .replace('{offer}', offerLink)
      .replace('{policy}', policyLink);
    consentLabelEl.querySelectorAll('a').forEach(a => {
      a.addEventListener('click', e => e.stopPropagation());
    });
  }
  if (consentEl && submitBtn) {
    submitBtn.disabled = !consentEl.checked;
    consentEl.addEventListener('change', () => {
      submitBtn.disabled = !consentEl.checked;
    });
  }
  if (form) {
    form.addEventListener('submit', async e => {
      e.preventDefault();

      // Reject free-text autocomplete values. A field with text but no ref means the
      // user typed without selecting a suggestion (or clicked submit before the blur-clear
      // fired). Clear it so the field's `required` attribute makes reportValidity() focus it.
      [
        ['order-city', 'order-city-ref'],
        ['order-novapost', 'order-novapost-ref'],
        ['order-address', 'order-address-ref'],
      ].forEach(([inputId, refId]) => {
        const input = document.getElementById(inputId);
        const ref = document.getElementById(refId);
        if (input && ref && !ref.value && input.value.trim()) input.value = '';
      });

      if (!form.checkValidity()) {
        form.reportValidity();
        return;
      }

      // Build CreateOrderRequest payload
      const fd = new FormData(form);
      const phoneCode = (document.getElementById('order-phone-code')?.textContent || '').trim();
      const phoneNumber = (fd.get('phone_number') || '').toString().trim();

      const attrs = product.attrs || {};
      const attrKeys = Object.keys(attrs);
      const attrValuesOrder = product.attr_values_order || {};
      const params = new URLSearchParams(window.location.search);
      const attributes = {};
      attrKeys.forEach(key => {
        const langData = attrs[key];
        if (!langData) return;
        const valKeys = attrValuesOrder[key] || Object.keys(langData.values);
        const fromUrl = params.get(key);
        attributes[key] = valKeys.includes(fromUrl) ? fromUrl : valKeys[0];
      });

      const delivery = fd.get('delivery');
      let address = '';
      if (delivery === 'courier') {
        const street = (fd.get('address') || '').toString().trim();
        const building = (fd.get('building') || '').toString().trim();
        const apt = (fd.get('apt') || '').toString().trim();
        address = [street, building, apt].filter(Boolean).join(', ');
      } else {
        address = (fd.get('novapost') || '').toString().trim();
      }

      const notes = (fd.get('comment') || '').toString().trim();
      const middleName = (fd.get('middlename') || '').toString().trim();

      const payload = {
        product_id: getProductId(),
        lang: currentLang,
        first_name: (fd.get('firstname') || '').toString().trim(),
        last_name: (fd.get('lastname') || '').toString().trim(),
        phone: phoneCode + phoneNumber,
        email: (fd.get('email') || '').toString().trim(),
        country: (fd.get('country') || '').toString().trim(),
        city: (fd.get('city') || '').toString().trim(),
        address,
      };
      if (Object.keys(attributes).length) payload.attributes = attributes;
      if (notes) payload.notes = notes;
      if (middleName) payload.middle_name = middleName;
      if (appliedPromo) payload.promo_code = appliedPromo.code;

      if (errorEl) { errorEl.textContent = ''; errorEl.style.display = 'none'; }
      if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.textContent = t('order.submitting');
      }

      try {
        const res = await createOrder(payload);
        if (res && res.payment_url) {
          window.location.href = res.payment_url;
          return; // keep button disabled while navigating
        }
        throw new Error(t('order.error'));
      } catch (err) {
        if (submitBtn) {
          submitBtn.disabled = consentEl ? !consentEl.checked : false;
          submitBtn.textContent = t('order.submit');
        }
        const promoErrKey = err && err.status === 400 &&
          { 'invalid promo code': 'order.promo.invalid', 'promo code already used': 'order.promo.used' }[err.message];
        if (promoErrKey) {
          appliedPromo = null;
          renderSummaryPrice();
          showPromoMsg(promoErrKey);
          if (promoInput) promoInput.focus();
          return;
        }
        if (errorEl) {
          errorEl.textContent = (err && err.message) || t('order.error');
          errorEl.style.display = '';
        }
      }
    });
  }
}

/* ---- Order Status ---- */

// Backend status → display bucket. Most map 1:1; the four pre-payment statuses collapse into
// `awaiting_payment` because the customer can't distinguish them in any meaningful way.
const ORDER_STATUS_BUCKETS = {
  new: 'awaiting_payment',
  awaiting_payment: 'awaiting_payment',
  payment_processing: 'awaiting_payment',
  payment_hold: 'awaiting_payment',
  paid: 'paid',
  processing: 'processing',
  cancelled: 'cancelled',
  shipped: 'shipped',
  delivered: 'delivered',
  refund_requested: 'refund_requested',
  returned: 'returned',
  refunded: 'refunded',
};
// Polling continues only while the bucket is the awaiting-payment one. Everything else is terminal
// from the page's POV — we don't expect customers to sit on the page through `processing → shipped`.
const ORDER_STATUS_POLL_BUCKET = 'awaiting_payment';
const ORDER_STATUS_POLL_MS = 3000;
const ORDER_STATUS_POLL_CAP = 40; // ~2 min total before timeout copy

function classifyOrderStatus(status) {
  return ORDER_STATUS_BUCKETS[status] || 'paid'; // unknown enum → neutral-positive, never `error`
}

function paintOrderStatus(view, bucket) {
  const spinner = view.querySelector('.payment-status-spinner');
  const heading = view.querySelector('.payment-status-heading');
  const note = view.querySelector('.payment-status-note');
  const back = view.querySelector('.payment-status-back');
  spinner.style.display = bucket === ORDER_STATUS_POLL_BUCKET ? '' : 'none';
  heading.textContent = t(`order.status.${bucket}.heading`);
  note.textContent = t(`order.status.${bucket}.note`);
  back.style.display = '';
  back.textContent = t('order.status.back');
}

// Set up the order-ID row once per render. Kept out of paintOrderStatus so that
// (a) the orderId text isn't rewritten on every poll tick, which would clobber
// the "Copied!" feedback mid-window, and (b) the click handler isn't re-stacked.
function setupOrderIdRow(view, orderId) {
  const row = view.querySelector('.payment-status-id');
  const label = row.querySelector('.payment-status-id-label');
  const oldBtn = row.querySelector('.payment-status-id-value');
  if (!orderId) {
    row.style.display = 'none';
    return;
  }
  row.style.display = '';
  label.textContent = t('order.status.id_label');
  // Clone-and-replace the button so a pending "Copied!" revert from a prior render
  // writes to a detached node and never flashes in the new render.
  const btn = oldBtn.cloneNode(false);
  btn.textContent = orderId;
  btn.title = t('order.status.copy_hint');
  oldBtn.parentNode.replaceChild(btn, oldBtn);
  btn.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(orderId);
    } catch (e) {
      return; // clipboard blocked (insecure context, permissions); silently skip
    }
    btn.textContent = t('order.status.copied');
    btn.classList.add('is-copied');
    setTimeout(() => {
      // Guard so a second click during the window doesn't double-revert.
      if (btn.classList.contains('is-copied')) {
        btn.textContent = orderId;
        btn.classList.remove('is-copied');
      }
    }, 1500);
  });
}

async function renderOrderStatus() {
  // Yield once so this resolves after applyShopContent's cached fetchShop await,
  // otherwise the shop-name title overwrites ours.
  await fetchShop().catch(() => {});
  document.title = `${t('order.status.title')}${titleSuffix()}`;

  const view = document.getElementById('order-status-view');
  if (!view) return;

  // Cancel any in-flight polling chain from a prior render (e.g. language switch).
  // Generation token also lets a tick currently between fetch and reschedule know
  // it's been superseded, so we never end up with two parallel chains.
  if (view._pollTimer) clearTimeout(view._pollTimer);
  view._pollTimer = null;
  const generation = Symbol('orderStatus');
  view._generation = generation;

  const orderId = new URLSearchParams(window.location.search).get('order_id');
  setupOrderIdRow(view, orderId);
  if (!orderId) {
    paintOrderStatus(view, 'error');
    return;
  }

  // Optimistic awaiting-payment copy so the page isn't blank during first fetch.
  paintOrderStatus(view, ORDER_STATUS_POLL_BUCKET);

  let polls = 0;
  const tick = async () => {
    if (view._generation !== generation) return;
    polls += 1;
    let res;
    try {
      res = await fetchOrderStatus(orderId);
    } catch (e) {
      if (view._generation !== generation) return;
      paintOrderStatus(view, 'error');
      return;
    }
    if (view._generation !== generation) return;
    const bucket = classifyOrderStatus(res.status);
    paintOrderStatus(view, bucket);
    if (bucket !== ORDER_STATUS_POLL_BUCKET) return;
    if (polls >= ORDER_STATUS_POLL_CAP) {
      paintOrderStatus(view, 'timeout');
      return;
    }
    view._pollTimer = setTimeout(tick, ORDER_STATUS_POLL_MS);
  };

  await tick();
}

/* ---- Markdown Pages ---- */

// First path segment of the URL, or null on the home page. index.html serves the
// home page, the catalog (/catalog) and every content page (/<page id>).
function getPageId() {
  const p = window.location.pathname.replace(/^\/|\/$/g, '');
  return p || null;
}

// Reserved route served by index.html's catalog view; a content page with this
// id would be shadowed.
const CATALOG_ROUTE = 'catalog';

async function renderMarkdownPage() {
  const container = document.getElementById('markdown-content');
  if (!container) return;
  const id = getPageId();
  if (!id) { container.innerHTML = '<p>Page not found.</p>'; return; }
  container.innerHTML = '<p class="offer-loading">…</p>';
  let [pages, md] = [null, null];
  try {
    [pages, md] = await Promise.all([fetchPages(), fetchPage(id, currentLang)]);
  } catch (e) {
    container.innerHTML = '<p>Failed to load content.</p>';
    return;
  }
  const meta = pages && pages.find(p => p.id === id);
  const title = meta ? loc(meta.title) : '';
  if (title) document.title = `${title}${titleSuffix()}`;
  const crumbs = document.getElementById('page-breadcrumbs');
  if (crumbs) {
    crumbs.innerHTML = `<a href="/">${esc(t('nav.home'))}</a><span aria-hidden="true">/</span><span aria-current="page">${esc(title || id)}</span>`;
  }
  let html = md ? marked.parse(md) : '';
  // The page title is shown in the band above the content; drop a leading H1 that repeats it.
  const tmp = document.createElement('div');
  tmp.innerHTML = html;
  const first = tmp.firstElementChild;
  const heading = document.getElementById('page-title');
  if (first && first.tagName === 'H1') {
    if (heading) heading.textContent = first.textContent;
    first.remove();
    html = tmp.innerHTML;
  } else if (heading) {
    heading.textContent = title || id;
  }
  container.innerHTML = html;
}

function hideLoader() {
  const el = document.getElementById('loader');
  if (!el) return;
  el.classList.add('loader-hidden');
  el.addEventListener('transitionend', () => { el.style.display = 'none'; }, { once: true });
}

/* ---- Init ---- */

document.addEventListener('DOMContentLoaded', async () => {
  // Route index.html: home at "/", the catalog at /catalog, content pages elsewhere.
  const homeView = document.getElementById('home-view');
  const catalogView = document.getElementById('catalog-view');
  const pageView = document.getElementById('page-view');
  if (homeView && pageView) {
    const route = getPageId();
    // Old links filtered the home grid with ?category=; the grid now lives in the catalog.
    if (!route && new URLSearchParams(window.location.search).get('category')) {
      window.location.replace(`/${CATALOG_ROUTE}${window.location.search}`);
      return;
    }
    homeView.style.display = route ? 'none' : '';
    if (catalogView) catalogView.style.display = route === CATALOG_ROUTE ? '' : 'none';
    pageView.style.display = route && route !== CATALOG_ROUTE ? '' : 'none';
  }

  // Ensure the loader is painted at least once before starting async work.
  // When navigating from another cached page, the entire DOMContentLoaded →
  // fetch → render cycle can complete before the browser's first paint, making
  // the loader invisible. Two rAF frames guarantee one painted frame with the
  // loader visible before we start fetching.
  await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));

  try {
    const shop = await fetchShop();
    availableLangs = Object.keys(shop.name || shop.title || shop.description || {});
  } catch (e) {
    availableLangs = ['en']; // minimal fallback if /shop is unreachable
  }
  if (availableLangs.length === 0) availableLangs = ['en'];
  currentLang = detectLang(availableLangs);
  applyAssets();
  applyAnalytics();
  applyI18n();
  applyShopContent();
  const renders = [renderHeader(), renderFooter()];
  if (homeView && homeView.style.display !== 'none') renders.push(renderHome());
  if (catalogView && catalogView.style.display !== 'none') renders.push(renderCatalog());
  const productView = document.getElementById('product-view');
  if (productView) {
    const isOrderView = Boolean(new URLSearchParams(window.location.search).get('order'));
    const orderView = document.getElementById('order-view');
    productView.style.display = isOrderView ? 'none' : '';
    if (orderView) orderView.style.display = isOrderView ? '' : 'none';
    if (isOrderView) renders.push(renderOrderView());
    else renders.push(renderProduct());
  }
  if (pageView && pageView.style.display !== 'none') renders.push(renderMarkdownPage());
  if (document.getElementById('order-status-view')) renders.push(renderOrderStatus());
  await Promise.all(renders);
  hideLoader();
});
