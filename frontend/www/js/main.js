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

// Reflect the selected home category into the URL (?category=<id>) so a copied link
// reopens on the same filter and the choice survives reload. Omitted for "All" (null)
// to keep URLs clean. Preserves any other params (e.g. ?lang=); drops the query string
// entirely when nothing is left so "All" is the bare pathname, not a trailing "?".
function syncCategoryUrl() {
  const p = new URLSearchParams(window.location.search);
  if (activeCategory) p.set('category', activeCategory);
  else p.delete('category');
  const qs = p.toString();
  history.replaceState(null, '', qs ? `?${qs}` : window.location.pathname);
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

async function applyShopContent() {
  try {
    const shop = await fetchShop();
    const name = (shop.name && shop.name[currentLang]) || (shop.title && shop.title[currentLang]);
    const title = shop.title && shop.title[currentLang];
    const desc = shop.description && shop.description[currentLang];
    if (name) document.title = name;
    if (title) {
      const heading = document.querySelector('.hero-heading');
      if (heading) heading.textContent = title;
    }
    if (desc) {
      const tagline = document.querySelector('.hero-tagline');
      if (tagline) tagline.textContent = desc;
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
  const asset = name => `${API_BASE}/assets/${name}`;
  const icons = [
    { rel: 'icon', href: asset('favicon.svg'), type: 'image/svg+xml' },
    { rel: 'icon', href: asset('favicon.ico'), sizes: 'any' },
    { rel: 'apple-touch-icon', href: asset('apple-touch-icon.png') },
  ];
  icons.forEach(spec => {
    const link = document.createElement('link');
    Object.entries(spec).forEach(([k, v]) => link.setAttribute(k, v));
    document.head.appendChild(link);
  });
  document.querySelectorAll('.footer-logo').forEach(img => { img.src = asset('logo.png'); });
}

// Map of social-link icon names (ShopLink.icon from /shop) to inline SVG markup.
// A link whose icon isn't here falls back to its title text.
const SOCIAL_ICONS = {
  instagram: '<svg xmlns="http://www.w3.org/2000/svg" width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="20" rx="5" ry="5"/><circle cx="12" cy="12" r="4.5"/><circle cx="17.5" cy="6.5" r="1" fill="currentColor" stroke="none"/></svg>',
};

// Render the footer social links from `shop.links[currentLang]` (array of
// {title, icon, url}). Re-runs on language switch. Empty when the shop provides
// no links for the active language.
function renderSocialLinks() {
  const container = document.getElementById('footer-social');
  if (!container) return;
  const shop = getCachedShop();
  const links = (shop && shop.links && shop.links[currentLang]) || [];
  container.innerHTML = links.map(l => {
    const inner = SOCIAL_ICONS[l.icon] || esc(l.title);
    return `<a href="${esc(l.url)}" target="_blank" rel="noopener" class="footer-social-link" aria-label="${esc(l.title)}">${inner}</a>`;
  }).join('');
}

// Trailing " — <shop name>" appended to document titles, derived from the cached
// /shop response. Empty (no suffix) until the shop loads or if it's unreachable.
function titleSuffix() {
  const shop = getCachedShop();
  const map = (shop && (shop.name || shop.title)) || {};
  const name = map[currentLang] || Object.values(map)[0] || '';
  return name ? ` — ${name}` : '';
}

function applyI18n() {
  document.querySelectorAll('[data-i18n]').forEach(el => {
    el.textContent = t(el.dataset.i18n);
  });
  document.documentElement.lang = currentLang;
}

function updateLangLinks() {
  availableLangs.forEach(lang => {
    const el = document.getElementById(`lang-${lang}`);
    if (el) el.classList.toggle('active', lang === currentLang);
  });
}

function initLangToggle() {
  availableLangs.forEach(lang => {
    const el = document.getElementById(`lang-${lang}`);
    if (!el) return;
    el.addEventListener('click', e => {
      e.preventDefault();
      currentLang = lang;
      localStorage.setItem(LANG_KEY, currentLang);
      applyI18n();
      updateLangLinks();
      applyShopContent();
      renderFooterLinks();
      renderSocialLinks();
      if (document.getElementById('product-grid')) renderHome();
      if (document.getElementById('product-view')) {
        const isOrderView = Boolean(new URLSearchParams(window.location.search).get('order'));
        if (isOrderView) renderOrderView();
        else renderProduct();
      }
      if (document.getElementById('markdown-content')) renderMarkdownPage();
      if (document.getElementById('order-status-view')) renderOrderStatus();
    });
  });
  updateLangLinks();
}

/* ---- Home Page ---- */

// null = "All". Module-level so it survives re-renders (e.g. language switch).
let activeCategory = null;

async function renderHome() {
  const grid = document.getElementById('product-grid');
  if (!grid) return;
  grid.innerHTML = '<p class="loading">…</p>';
  let products, shop;
  try {
    [products, shop] = await Promise.all([fetchProducts(), fetchShop()]);
  } catch (e) {
    grid.innerHTML = '<p class="loading">Failed to load products.</p>';
    return;
  }
  renderCategoryBar(shop, products);
  renderProductGrid(products);
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
    categories.map(c => ({id: c.id, label: (c.title && c.title[currentLang]) || c.id}))
  );
  // innerHTML rewrite discards old listeners, so no clone-and-replace needed here.
  bar.innerHTML = tabs.map(tab =>
    `<button class="category-tab${tab.id === activeCategory ? ' active' : ''}" data-cat="${esc(tab.id || '')}">${esc(tab.label)}</button>`
  ).join('');
  bar.querySelectorAll('.category-tab').forEach(btn => {
    btn.addEventListener('click', () => {
      activeCategory = btn.dataset.cat || null;
      syncCategoryUrl();
      bar.querySelectorAll('.category-tab').forEach(b => b.classList.toggle('active', b === btn));
      renderProductGrid(products);
    });
  });
}

function renderProductGrid(products) {
  const grid = document.getElementById('product-grid');
  if (!grid) return;
  const filtered = activeCategory
    ? products.filter(p => Array.isArray(p.categories) && p.categories.includes(activeCategory))
    : products;
  grid.innerHTML = filtered.map(p => {
    const imgSrc = p.image ? API_BASE + p.image : '';
    return `
      <a class="card" href="/product?id=${esc(p.id)}">
        ${imgSrc ? `<img class="card-image" src="${esc(imgSrc)}" alt="${esc(p.title[currentLang] || '')}">` : ''}
        <div class="card-body">
          <div class="card-name">${esc(p.title[currentLang] || '')}</div>
          <div class="card-tagline">${esc(productTagline(p, currentLang))}</div>
          <div class="card-link">${esc(t('card.view'))}</div>
        </div>
      </a>
    `;
  }).join('');
}

/* ---- Product Page ---- */

function getProductId() {
  return new URLSearchParams(window.location.search).get('id');
}

function openLightbox(src, alt, type) {
  const lb = document.createElement('div');
  lb.className = 'lightbox';
  const media = type === 'video'
    ? `<video src="${esc(src)}" autoplay loop muted playsinline></video>`
    : `<img src="${esc(src)}" alt="${esc(alt)}">`;
  lb.innerHTML = `<button class="lightbox-close" aria-label="Close">&#215;</button>${media}`;
  const close = () => { lb.remove(); document.body.style.overflow = ''; document.removeEventListener('keydown', onKey); };
  lb.addEventListener('click', close);
  document.body.appendChild(lb);
  document.body.style.overflow = 'hidden';
  const onKey = e => { if (e.key === 'Escape') close(); };
  document.addEventListener('keydown', onKey);
}

function renderCarousel(container, images, altText) {
  const multi = images.length > 1;

  const slideHtml = img => img.type === 'video'
    ? `<video class="carousel-slide" src="${esc(img.preview)}" loop muted playsinline preload="auto"></video>`
    : `<img class="carousel-slide" src="${esc(img.preview)}" alt="${esc(altText)}">`;

  container.innerHTML = `
    <div class="carousel-track">
      ${images.map(slideHtml).join('')}
    </div>
    ${multi ? `
      <button class="carousel-btn carousel-prev">&#8249;</button>
      <button class="carousel-btn carousel-next">&#8250;</button>
      <div class="carousel-dots">
        ${images.map((_, i) => `<button class="carousel-dot${i === 0 ? ' active' : ''}" data-index="${i}"></button>`).join('')}
      </div>
    ` : ''}
  `;

  const slides = container.querySelectorAll('.carousel-slide');
  slides.forEach((slide, i) => {
    slide.style.cursor = 'zoom-in';
    slide.addEventListener('click', () => openLightbox(images[i].full, altText, images[i].type));
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
  const track = container.querySelector('.carousel-track');
  const dots = container.querySelectorAll('.carousel-dot');

  function goTo(index) {
    current = (index + images.length) % images.length;
    track.style.transform = `translateX(-${current * 100}%)`;
    dots.forEach((d, i) => d.classList.toggle('active', i === current));
    syncVideoPlayback(current);
    syncCarouselUrl(current);
  }

  container.querySelector('.carousel-prev').addEventListener('click', () => goTo(current - 1));
  container.querySelector('.carousel-next').addEventListener('click', () => goTo(current + 1));
  dots.forEach(d => d.addEventListener('click', () => goTo(Number(d.dataset.index))));

  container._goTo = goTo;

  let touchStartX = null;
  let dragging = false;

  track.addEventListener('touchstart', e => {
    touchStartX = e.touches[0].clientX;
    dragging = false;
    track.style.transition = 'none';
  }, { passive: true });

  track.addEventListener('touchmove', e => {
    if (touchStartX === null) return;
    dragging = true;
    const delta = e.touches[0].clientX - touchStartX;
    const w = container.offsetWidth;
    track.style.transform = `translateX(${-(current * w) + delta}px)`;
  }, { passive: true });

  track.addEventListener('touchend', e => {
    if (touchStartX === null) return;
    track.style.transition = '';
    if (dragging) {
      const delta = e.changedTouches[0].clientX - touchStartX;
      if (Math.abs(delta) > container.offsetWidth * 0.25) {
        goTo(current + (delta < 0 ? 1 : -1));
      } else {
        track.style.transform = `translateX(-${current * 100}%)`;
      }
    }
    touchStartX = null;
    dragging = false;
  }, { passive: true });
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
      return `
        <div class="variant-group">
          <div class="variant-name">${esc(langData.title)}</div>
          <div class="variant-options">
            ${(attrValuesOrder[key] || Object.keys(langData.values)).map(valKey => {
              const val = langData.values[valKey];
              if (!val) return '';
              const label = val.prefix ? `${val.prefix} ${val.title}` : val.title;
              return `
              <button class="variant-option${selections[key] === valKey ? ' active' : ''}"
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
    const newBtn = orderBtn.cloneNode(true);
    orderBtn.parentNode.replaceChild(newBtn, orderBtn);
    newBtn.addEventListener('click', () => {
      const p = new URLSearchParams(window.location.search);
      p.set('order', '1');
      window.location.search = p.toString();
    });
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

  // Restore the carousel slide from the URL (URL wins on load). Variant init never
  // moves the carousel — only variant clicks do — so the URL is the sole authority here.
  const imgIdx = parseInt(new URLSearchParams(window.location.search).get('img'), 10);
  if (carouselEl && carouselEl._goTo && imgIdx > 0 && imgIdx < (product.images || []).length) {
    carouselEl._goTo(imgIdx);
  }
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

    const attrsHtml = attrKeys.map(key => {
      const langData = attrs[key];
      if (!langData) return '';
      const valKey = selections[key];
      const val = langData.values[valKey];
      const valTitle = (val && val.title) || valKey;
      const valLabel = (val && val.prefix) ? `${val.prefix} ${valTitle}` : valTitle;
      const descHtml = langData.description ? `<div class="order-summary-attr-desc">${marked.parse(langData.description)}</div>` : '';
      return `<div class="order-summary-attr"><span class="order-summary-attr-label">${esc(langData.title)}:</span> ${esc(valLabel)}${descHtml}</div>`;
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

function getPageId() {
  const p = window.location.pathname.replace(/^\/|\/$/g, '');
  return p || null;
}

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
  if (meta && meta.title) document.title = `${meta.title[currentLang] || meta.title}${titleSuffix()}`;
  container.innerHTML = md ? marked.parse(md) : '';
}

/* ---- Footer Links ---- */

async function renderFooterLinks() {
  const container = document.getElementById('footer-links');
  if (!container) return;
  let pages;
  try {
    pages = await fetchPages();
  } catch (e) {
    return;
  }
  container.innerHTML = pages.map((p, i) => {
    const title = (p.title && p.title[currentLang]) || p.id;
    return (i > 0 ? '<span class="footer-lang-sep">·</span>' : '') +
      `<a href="/${esc(p.id)}">${esc(title)}</a>`;
  }).join('');
}

function hideLoader() {
  const el = document.getElementById('loader');
  if (!el) return;
  el.classList.add('loader-hidden');
  el.addEventListener('transitionend', () => { el.style.display = 'none'; }, { once: true });
}

/* ---- Init ---- */

document.addEventListener('DOMContentLoaded', async () => {
  // Route index.html: show page view when URL path is not root
  const homeView = document.getElementById('home-view');
  const pageView = document.getElementById('page-view');
  if (homeView && pageView) {
    const isPageView = Boolean(getPageId());
    homeView.style.display = isPageView ? 'none' : '';
    pageView.style.display = isPageView ? '' : 'none';
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
  initLangToggle();
  renderFooterLinks();
  renderSocialLinks();
  const renders = [];
  if (document.getElementById('product-grid')) renders.push(renderHome());
  const productView = document.getElementById('product-view');
  if (productView) {
    const isOrderView = Boolean(new URLSearchParams(window.location.search).get('order'));
    const orderView = document.getElementById('order-view');
    productView.style.display = isOrderView ? 'none' : '';
    if (orderView) orderView.style.display = isOrderView ? '' : 'none';
    if (isOrderView) renders.push(renderOrderView());
    else renders.push(renderProduct());
  }
  if (document.getElementById('markdown-content')) renders.push(renderMarkdownPage());
  if (document.getElementById('order-status-view')) renders.push(renderOrderStatus());
  await Promise.all(renders);
  hideLoader();
});
