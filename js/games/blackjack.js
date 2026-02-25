  // ── Blackjack UI ───────────────────────────────────────────────────────────
  function showBlackjackUI() {
    switchGameView('blackjack');
  }

  /** 블랙잭 하트 바 HTML (인디언 포커 스타일 재활용) */
  function renderBJHeartsBar(count) {
    const MAX_ICONS = 20;
    const displayMax = Math.max(10, count);
    let html = '';
    const iconCount = Math.min(count, MAX_ICONS);
    const totalSlots = Math.min(displayMax, MAX_ICONS);
    for (let i = 0; i < totalSlots; i++) {
      html += `<span class="indian-heart${i >= iconCount ? ' lost' : ''}">❤️</span>`;
    }
    if (count > MAX_ICONS) {
      html += `<span class="indian-hearts-count">+${count - MAX_ICONS}</span>`;
    }
    return html;
  }

  function renderBlackjackState(data) {
    if (!data) return;
    renderBJHand('bj-dealer-hand', 'bj-dealer-score', data.dealerHand);
    renderBJHand('bj-player-hand', 'bj-player-score', data.playerHand);

    const dealerHeartsEl = document.getElementById('bj-dealer-hearts');
    const playerHeartsEl = document.getElementById('bj-player-hearts');
    if (dealerHeartsEl) dealerHeartsEl.innerHTML = renderBJHeartsBar(data.dealerHearts ?? 10);
    if (playerHeartsEl) playerHeartsEl.innerHTML = renderBJHeartsBar(data.playerHearts ?? 10);

    const msgEl = document.getElementById('bj-message');
    const overlayEl = document.getElementById('bj-result-overlay');
    const boxEl = document.getElementById('bj-result-box');
    const msgTextEl = document.getElementById('bj-result-msg');
    if (msgTextEl) msgTextEl.textContent = data.message || '';
    if (data.message) msgEl.textContent = data.message;

    const showResult = (data.phase === 'settlement' || data.phase === 'game_over') && data.message;
    if (showResult) {
      if (msgTextEl) msgTextEl.textContent = data.message;
      boxEl.className = 'unified-result-box';
      if (data.phase === 'game_over' && data.gameOverPlayerWin) {
        boxEl.classList.add('win');
      } else if (data.phase === 'game_over') {
        boxEl.classList.add('lose');
      } else if (/승리|Win|이겼|블랙잭/i.test(data.message)) {
        boxEl.classList.add('win');
      } else if (/패배|Lose|졌|버스트|Bust/i.test(data.message)) {
        boxEl.classList.add('lose');
      } else {
        boxEl.classList.add('push');
      }

      overlayEl.style.display = 'flex';

      if (window.bjResultTimer) clearTimeout(window.bjResultTimer);
      const isGameOver = data.phase === 'game_over';
      window.bjResultTimer = setTimeout(() => {
        overlayEl.style.display = 'none';
      }, isGameOver ? 5000 : 3500);
    } else {
      overlayEl.style.display = 'none';
    }

    const startBtns    = document.getElementById('bj-start-buttons');
    const gameBtns     = document.getElementById('bj-game-buttons');
    const spectatorMsg = document.getElementById('bj-spectator-msg');

    // 관전자 판별: mainPlayerId가 있고 나와 다르면 관전자
    const isSpectator = data.mainPlayerId && data.mainPlayerId !== currentUserId;
    if (isSpectator) {
      startBtns.style.display    = 'none';
      gameBtns.style.display     = 'none';
      spectatorMsg.style.display = 'block';
    } else {
      spectatorMsg.style.display = 'none';
      if (data.phase === 'game_over') {
        startBtns.style.display = 'none';
        gameBtns.style.display = 'none';
      } else if (data.phase === 'betting' || data.phase === 'settlement') {
        startBtns.style.display = 'flex'; gameBtns.style.display = 'none';
        const rematchArea = document.getElementById('rematch-area');
        if (rematchArea) { rematchArea.style.display = 'none'; rematchArea.classList.remove('visible'); }
      } else if (data.phase === 'player_turn') {
        startBtns.style.display = 'none'; gameBtns.style.display = 'flex';
      } else {
        startBtns.style.display = 'none'; gameBtns.style.display = 'none';
      }
    }
  }

  function renderBJHand(handElId, scoreElId, handInfo) {
    const handEl  = document.getElementById(handElId);
    const scoreEl = document.getElementById(scoreElId);
    if (!handInfo || !handInfo.cards || handInfo.cards.length === 0) {
      handEl.innerHTML = ''; scoreEl.textContent = ''; scoreEl.className = 'bj-score'; return;
    }
    handEl.innerHTML = handInfo.cards.map(card => {
      if (card.hidden) return `<div class="bj-card hidden"></div>`;
      const isRed = card.suit === '♥' || card.suit === '♦';
      return `<div class="bj-card ${isRed ? 'red' : 'black'}">
        <span class="bj-card-top">${card.value}</span>
        <span class="bj-card-center">${card.suit}</span>
        <span class="bj-card-bot">${card.value}</span>
      </div>`;
    }).join('');
    const score = handInfo.score;
    scoreEl.textContent = score > 0 ? `${score}` : '';
    scoreEl.className   = score > 21 ? 'bj-score bust' : 'bj-score';
  }

  function bjStart() {
    if (!ws || ws.readyState !== WebSocket.OPEN || !currentRoomId) return;
    sendGameAction({ cmd: 'start' });
  }
  function bjHit() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'hit' });
  }
  function bjStand() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'stand' });
  }