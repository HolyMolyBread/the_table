  // ── Blackjack UI ───────────────────────────────────────────────────────────
  function showBlackjackUI() {
    switchGameView('blackjack');
  }

  function renderBlackjackState(data) {
    if (!data) return;
    renderBJHand('bj-dealer-hand', 'bj-dealer-score', data.dealerHand);
    renderBJHand('bj-player-hand', 'bj-player-score', data.playerHand);
    const msgEl = document.getElementById('bj-message');
    const overlayEl = document.getElementById('bj-result-overlay');
    const boxEl = document.getElementById('bj-result-box');
    const msgTextEl = document.getElementById('bj-result-msg');
    if (data.message) msgEl.textContent = data.message;

    const showResult = (data.phase === 'settlement' || data.state === 'game_over') && data.message;
    if (showResult) {
      msgTextEl.textContent = data.message;
      boxEl.className = 'unified-result-box';
      if (/승리|Win|이겼|블랙잭/i.test(data.message)) boxEl.classList.add('win');
      else if (/패배|Lose|졌|버스트|Bust/i.test(data.message)) boxEl.classList.add('lose');
      else boxEl.classList.add('push');

      overlayEl.style.display = 'flex';

      if (window.bjResultTimer) clearTimeout(window.bjResultTimer);
      window.bjResultTimer = setTimeout(() => {
        overlayEl.style.display = 'none';
      }, 3500);
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
      if (data.phase === 'betting' || data.phase === 'settlement') {
        startBtns.style.display = 'flex'; gameBtns.style.display = 'none';
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