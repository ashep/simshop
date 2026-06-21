// js/api.js
// API base: set `window.API_BASE` before loading this script to point at a
// separate API host; otherwise it defaults to `/api` on the current origin.
const API_BASE = window.API_BASE || (window.location.origin + '/api');

let _shopCache = null;

async function fetchShop() {
    if (_shopCache) return _shopCache;
    const res = await fetch(`${API_BASE}/shop`);
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    _shopCache = await res.json();
    return _shopCache;
}

// Synchronous access to the cached /shop response (null until fetchShop resolves).
function getCachedShop() {
    return _shopCache;
}

let _productsCache = null;

async function fetchProducts() {
    if (_productsCache) return _productsCache;
    const res = await fetch(`${API_BASE}/products`);
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    _productsCache = await res.json();
    return _productsCache;
}

let _pagesCache = null;

async function fetchPages() {
    if (_pagesCache) return _pagesCache;
    const res = await fetch(`${API_BASE}/pages`);
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    _pagesCache = await res.json();
    return _pagesCache;
}

async function fetchPage(id, lang) {
    const res = await fetch(`${API_BASE}/pages/${id}/${lang}`);
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    return res.text();
}

const _productCache = {};

async function fetchProduct(id, lang) {
    const key = `${id}/${lang}`;
    if (_productCache[key]) return _productCache[key];
    const res = await fetch(`${API_BASE}/products/${id}/${lang}`);
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    _productCache[key] = await res.json();
    return _productCache[key];
}

async function fetchNovaPoshtaCities(q) {
    const res = await fetch(`${API_BASE}/nova-poshta/cities?q=${encodeURIComponent(q)}`);
    if (!res.ok) return [];
    return res.json();
}

async function fetchNovaPoshtaBranches(cityRef, q) {
    const res = await fetch(`${API_BASE}/nova-poshta/branches?city_ref=${encodeURIComponent(cityRef)}&q=${encodeURIComponent(q)}`);
    if (!res.ok) return [];
    return res.json();
}

async function fetchNovaPoshtaStreets(cityRef, q) {
    const res = await fetch(`${API_BASE}/nova-poshta/streets?city_ref=${encodeURIComponent(cityRef)}&q=${encodeURIComponent(q)}`);
    if (!res.ok) return [];
    return res.json();
}

async function fetchOrderStatus(id) {
    const res = await fetch(`${API_BASE}/orders/${encodeURIComponent(id)}`);
    if (!res.ok) {
        let msg = `API error: ${res.status}`;
        try {
            const body = await res.json();
            if (body && body.error) msg = body.error;
        } catch (_) { /* non-JSON body */
        }
        const err = new Error(msg);
        err.status = res.status;
        throw err;
    }
    return res.json();
}

async function createOrder(payload) {
    const res = await fetch(`${API_BASE}/orders`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload),
    });
    if (!res.ok) {
        let msg = `API error: ${res.status}`;
        try {
            const body = await res.json();
            if (body && body.error) msg = body.error;
        } catch (_) { /* non-JSON body */
        }
        const err = new Error(msg);
        err.status = res.status;
        throw err;
    }
    return res.json();
}
