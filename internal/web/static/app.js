// GW2 Raid Bingo single-page app. Vanilla JS, no build step. All server text is
// inserted with textContent (never innerHTML), so card texts cannot inject HTML.
'use strict';

var state = {
  me: null,
  guilds: [],
  guild: null,      // {id, name, admin}
  instance: null,   // instance key
  board: null,      // board response
  es: null          // EventSource
};

var INSTANCES = [
  ['w1', 'W1'], ['w2', 'W2'], ['w3', 'W3'], ['w4', 'W4'],
  ['w5', 'W5'], ['w6', 'W6'], ['w7', 'W7'], ['w8', 'W8'], ['htcm', 'HTCM']
];

var app = document.getElementById('app');
var accountEl = document.getElementById('account');

// --- helpers ---
function el(tag, opts) {
  var e = document.createElement(tag);
  opts = opts || {};
  if (opts.class) e.className = opts.class;
  if (opts.text != null) e.textContent = opts.text;
  if (opts.onclick) e.addEventListener('click', opts.onclick);
  if (opts.attrs) for (var k in opts.attrs) e.setAttribute(k, opts.attrs[k]);
  return e;
}
function clear(node) { while (node.firstChild) node.removeChild(node.firstChild); }

async function api(method, path, body) {
  var opts = { method: method, headers: {} };
  if (body) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
  var res = await fetch(path, opts);
  var data = null;
  try { data = await res.json(); } catch (e) { /* no body */ }
  if (!res.ok) { throw new Error((data && data.error) || ('request failed (' + res.status + ')')); }
  return data;
}

// --- boot ---
async function boot() {
  state.me = await api('GET', '/api/me');
  renderAccount();
  if (!state.me.loggedIn) { renderLanding(); return; }
  await loadGuilds();
}

function avatarUrl(u) {
  if (u && u.avatar) return 'https://cdn.discordapp.com/avatars/' + u.id + '/' + u.avatar + '.png?size=64';
  return 'https://cdn.discordapp.com/embed/avatars/0.png';
}

function renderAccount() {
  clear(accountEl);
  if (state.me && state.me.loggedIn) {
    accountEl.appendChild(el('img', { class: 'avatar', attrs: { src: avatarUrl(state.me.user), alt: '' } }));
    // Once a server is selected, show its name; otherwise the Discord username.
    var label = state.guild ? state.guild.name : '@' + state.me.user.username;
    accountEl.appendChild(el('span', { class: 'acct-label', text: label }));
    accountEl.appendChild(el('button', { class: 'btn secondary', text: 'Log out', onclick: logout }));
  } else if (state.me && state.me.loginEnabled) {
    accountEl.appendChild(el('a', { class: 'btn', text: 'Log in with Discord', attrs: { href: '/auth/login' } }));
  }
}

async function logout() {
  await fetch('/auth/logout', { method: 'POST' });
  location.href = '/';
}

// --- logged-out view: log in to play (the marketing lives on the bot site) ---
function renderLanding() {
  clear(app);
  var hero = el('div', { class: 'hero' });
  hero.appendChild(el('img', { class: 'crest-lg', attrs: { src: '/assets/logo.png', alt: '' } }));
  hero.appendChild(el('h1', { text: 'GW2 Raid Bingo' }));
  hero.appendChild(el('p', { text: 'Log in with Discord to play your bingo card, in sync with your Discord game.' }));
  if (state.me && state.me.loginEnabled) {
    hero.appendChild(el('a', { class: 'btn', text: 'Log in with Discord', attrs: { href: '/auth/login' } }));
  } else {
    hero.appendChild(el('p', { class: 'muted', text: 'Login is not configured on this server.' }));
  }
  if (state.me && state.me.botUrl) {
    hero.appendChild(el('a', { class: 'sub-link', text: 'New here? Add the bot to your server →', attrs: { href: state.me.botUrl } }));
  }
  app.appendChild(hero);
}

// --- guild selection (multi-server handled here) ---
async function loadGuilds() {
  clear(app);
  app.appendChild(el('p', { class: 'loading', text: 'Loading your servers...' }));
  try {
    var data = await api('GET', '/api/guilds');
    state.guilds = data.guilds || [];
  } catch (e) {
    clear(app); app.appendChild(el('p', { class: 'error', text: e.message })); return;
  }
  if (state.guilds.length === 0) { renderNoGuilds(); return; }
  if (state.guilds.length === 1) { selectGuild(state.guilds[0]); return; }
  renderGuildPicker();
}

function renderNoGuilds() {
  clear(app);
  var panel = el('div', { class: 'panel' });
  panel.appendChild(el('h2', { text: 'No shared servers yet' }));
  panel.appendChild(el('p', { class: 'muted', text: 'The bot is not in any server you are in. Add it to a server to start playing.' }));
  panel.appendChild(el('a', { class: 'btn', text: 'Add the bot to your server', attrs: { href: '/invite' } }));
  app.appendChild(panel);
}

function renderGuildPicker() {
  state.guild = null;
  renderAccount(); // back to the username until a server is picked
  clear(app);
  var panel = el('div', { class: 'panel' });
  panel.appendChild(el('h2', { text: 'Choose a server' }));
  panel.appendChild(el('p', { class: 'muted', text: 'You can play GW2 Raid Bingo in these servers:' }));
  state.guilds.forEach(function (g) {
    var item = el('button', { class: 'picker-item', onclick: function () { selectGuild(g); } });
    if (g.icon) {
      item.appendChild(el('img', { class: 'guild-icon', attrs: { src: 'https://cdn.discordapp.com/icons/' + g.id + '/' + g.icon + '.png?size=64', alt: '' } }));
    } else {
      item.appendChild(el('span', { class: 'guild-icon' }));
    }
    item.appendChild(el('span', { text: g.name }));
    if (g.admin) item.appendChild(el('span', { class: 'badge', text: 'admin' }));
    panel.appendChild(item);
  });
  app.appendChild(panel);
}

function selectGuild(g) {
  state.guild = g;
  state.instance = state.instance || 'w1';
  renderAccount(); // show the selected server name + avatar in the header
  renderGame();
}

// --- game view ---
function renderGame() {
  clear(app);
  closeStream();

  if (state.guilds.length > 1) {
    var back = el('button', { class: 'btn secondary', text: '← Change server', onclick: renderGuildPicker });
    app.appendChild(back);
  }
  var head = el('div', { class: 'row' });
  head.appendChild(el('h2', { text: state.guild.name }));
  app.appendChild(head);

  var tabs = el('div', { class: 'tabs' });
  INSTANCES.forEach(function (pair) {
    var t = el('button', { class: 'tab' + (pair[0] === state.instance ? ' active' : ''), text: pair[1],
      onclick: function () { state.instance = pair[0]; renderGame(); } });
    tabs.appendChild(t);
  });
  app.appendChild(tabs);

  var container = el('div', { attrs: { id: 'board-container' } });
  app.appendChild(container);
  loadBoard();
  openStream();
}

async function loadBoard() {
  var container = document.getElementById('board-container');
  if (!container) return;
  try {
    state.board = await api('GET', '/api/guild/' + state.guild.id + '/board?instance=' + state.instance);
  } catch (e) {
    clear(container); container.appendChild(el('p', { class: 'error', text: e.message })); return;
  }
  renderBoard();
}

var lastWinShown = false;
function renderBoard() {
  var container = document.getElementById('board-container');
  if (!container) return;
  clear(container);
  var b = state.board;

  if (!b.game) {
    var panel = el('div', { class: 'panel' });
    panel.appendChild(el('p', { class: 'muted', text: 'No game is open for this instance.' }));
    if (b.admin) panel.appendChild(el('button', { class: 'btn', text: 'Open a game', onclick: newGame }));
    else panel.appendChild(el('p', { class: 'muted', text: 'Ask a bingo admin to open one with /bingo new.' }));
    container.appendChild(panel);
    return;
  }

  var info = el('div', { class: 'panel' });
  var infoRow = el('div', { class: 'row' });
  infoRow.appendChild(el('span', { text: 'Game #' + b.game.id + ' · ' + (b.players || 0) + ' players' }));
  infoRow.appendChild(el('span', { class: 'spacer' }));
  if (b.admin) {
    infoRow.appendChild(el('button', { class: 'btn secondary', text: 'New game', onclick: newGame }));
    infoRow.appendChild(el('button', { class: 'btn danger', text: 'Abort', onclick: abortGame }));
  }
  info.appendChild(infoRow);
  container.appendChild(info);

  if (!b.card) {
    var dealPanel = el('div', { class: 'panel' });
    dealPanel.appendChild(el('p', { class: 'muted', text: 'You have not joined this game yet.' }));
    dealPanel.appendChild(el('button', { class: 'btn', text: 'Deal me a card', onclick: dealCard }));
    container.appendChild(dealPanel);
  } else {
    renderCard(container, b.card, b.hasBingo);
  }

  if (b.leaders && b.leaders.length) {
    var lead = el('div', { class: 'panel' });
    lead.appendChild(el('strong', { text: 'Closest to bingo' }));
    var ul = el('ul', { class: 'leaders' });
    b.leaders.slice(0, 5).forEach(function (p, i) {
      var li = el('li');
      li.appendChild(el('span', { text: (i + 1) + '. ' + (p.userId === state.me.user.id ? 'You' : 'Player ' + p.userId.slice(-4)) }));
      li.appendChild(el('span', { text: p.bestLine + '/5 (' + p.marked + ' marked)' }));
      ul.appendChild(li);
    });
    lead.appendChild(ul);
    container.appendChild(lead);
  }
}

function renderCard(container, card, hasBingo) {
  var playable = state.board.game.status === 'open';
  var letters = ['B', 'I', 'N', 'G', 'O'];
  var byIndex = {};
  card.cells.forEach(function (c) { byIndex[c.index] = c; });

  // A 6x6 grid: an empty corner + BINGO across the top, then each row is its
  // BINGO letter followed by the row's five cells - framing the card both ways.
  var grid = el('div', { class: 'board-grid' });
  grid.appendChild(el('div', { class: 'bl corner' }));
  letters.forEach(function (l) { grid.appendChild(el('div', { class: 'bl', text: l })); });
  for (var r = 0; r < 5; r++) {
    grid.appendChild(el('div', { class: 'bl', text: letters[r] }));
    for (var col = 0; col < 5; col++) {
      var idx = r * 5 + col;
      var cell = byIndex[idx];
      if (!cell) continue;
      var cls = 'cell' + (cell.free ? ' free' : cell.marked ? ' marked' : '') + (playable && !cell.free ? ' playable' : '');
      var cEl = el('div', { class: cls, text: cell.text });
      if (playable && !cell.free) {
        (function (index) { cEl.addEventListener('click', function () { toggle(card.id, index); }); })(idx);
      }
      grid.appendChild(cEl);
    }
  }
  container.appendChild(grid);

  if (hasBingo && playable) {
    var callPanel = el('div', { class: 'panel' });
    callPanel.style.textAlign = 'center';
    callPanel.style.marginTop = '0.8rem';
    callPanel.appendChild(el('button', { class: 'btn gold', text: 'CALL BINGO!', onclick: function () { callBingo(card.id); } }));
    container.appendChild(callPanel);
  }
}

// --- actions ---
async function dealCard() {
  try { await api('POST', '/api/guild/' + state.guild.id + '/card', { instance: state.instance }); await loadBoard(); }
  catch (e) { alert(e.message); }
}
// Client-side bingo check, mirroring the server (rows, columns, both diagonals;
// the free centre always counts). Used for the instant, optimistic UI - the
// backend re-validates, so this is only for responsiveness.
function hasBingoClient(cells) {
  var m = new Array(25);
  for (var i = 0; i < 25; i++) m[i] = false;
  cells.forEach(function (c) { if (c.index >= 0 && c.index < 25 && (c.marked || c.free)) m[c.index] = true; });
  m[12] = true; // free centre
  var lines = [];
  for (var r = 0; r < 5; r++) { var row = []; for (var c = 0; c < 5; c++) row.push(r * 5 + c); lines.push(row); }
  for (var col = 0; col < 5; col++) { var cl = []; for (var rr = 0; rr < 5; rr++) cl.push(rr * 5 + col); lines.push(cl); }
  lines.push([0, 6, 12, 18, 24]);
  lines.push([4, 8, 12, 16, 20]);
  return lines.some(function (line) { return line.every(function (idx) { return m[idx]; }); });
}

// toggle marks a cell optimistically (colour + CALL BINGO appear instantly), then
// sends to the backend and reverts if it is rejected. The SSE event the write
// produces reconciles the board with the server's authoritative state.
function toggle(cardId, index) {
  var b = state.board;
  if (!b || !b.card) return;
  var cell = null;
  for (var i = 0; i < b.card.cells.length; i++) { if (b.card.cells[i].index === index) { cell = b.card.cells[i]; break; } }
  if (!cell || cell.free) return;

  var prev = cell.marked;
  cell.marked = !prev;
  b.hasBingo = hasBingoClient(b.card.cells);
  renderBoard();

  api('POST', '/api/guild/' + state.guild.id + '/toggle', { cardId: cardId, index: index })
    .catch(function (e) {
      cell.marked = prev;
      b.hasBingo = hasBingoClient(b.card.cells);
      renderBoard();
      alert(e.message);
    });
}
async function callBingo(cardId) {
  try {
    await api('POST', '/api/guild/' + state.guild.id + '/call', { cardId: cardId });
    if (window.burstConfetti) window.burstConfetti();
    await loadBoard();
  } catch (e) { alert(e.message); }
}
async function newGame() {
  var replace = state.board && state.board.game ? confirm('A game is open. Replace it? Its cards become read-only.') : false;
  try { await api('POST', '/api/guild/' + state.guild.id + '/game/new', { instance: state.instance, replace: replace }); await loadBoard(); }
  catch (e) { alert(e.message); }
}
async function abortGame() {
  if (!confirm('Abort the open game? Its cards become read-only.')) return;
  try { await api('POST', '/api/guild/' + state.guild.id + '/game/abort', { instance: state.instance }); await loadBoard(); }
  catch (e) { alert(e.message); }
}

// --- live updates ---
function openStream() {
  closeStream();
  state.es = new EventSource('/api/guild/' + state.guild.id + '/events?instance=' + state.instance);
  state.es.onmessage = handleStreamEvent;
  ['game_opened', 'game_finished', 'game_aborted', 'card_dealt', 'cell_toggled'].forEach(function (k) {
    state.es.addEventListener(k, handleStreamEvent);
  });
}
function closeStream() { if (state.es) { state.es.close(); state.es = null; } }

var winCelebrated = false;
async function handleStreamEvent(ev) {
  var kind = '';
  try { kind = JSON.parse(ev.data).kind; } catch (e) {}
  await loadBoard();
  if (kind === 'game_finished' && !winCelebrated) {
    winCelebrated = true;
    if (window.burstConfetti) window.burstConfetti();
    setTimeout(function () { winCelebrated = false; }, 5000);
  }
}

boot().catch(function (e) {
  clear(app);
  app.appendChild(el('p', { class: 'error', text: e.message }));
});
