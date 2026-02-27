  // ── Texas Holdem UI ───────────────────────────────────────────────────────

  function showHoldemUI() {
    switchGameView('holdem');
  }

  function renderHoldemCard(card) {
    if (!card || card.hidden) {
      return `<div class="holdem-card hidden">🃏</div>`;
    }
    const isRed = card.suit === '♥' || card.suit === '♦';
    return `<div class="holdem-card ${isRed ? 'red-suit' : 'black-suit'}">
      <span class="holdem-card-top">${card.value}</span>
      <span class="holdem-card-suit">${card.suit}</span>
      <span class="holdem-card-bot">${card.value}</span>
    </div>`;
  }

  function renderHoldem(data) {
    if (!data) return;

    const isMyTurn = data.currentTurn === currentUserId;
    const isPlayer = data.players && data.players.some(p => p.userId === currentUserId);

    document.getElementById('holdem-round-bar').textContent = `라운드 ${data.round || 0}`;
    document.getElementById('holdem-pot-bar').textContent = `팟 ⭐×${data.pot || 0}`;

    const communityEl = document.getElementById('holdem-community-cards');
    if (communityEl && data.communityCards) {
      communityEl.innerHTML = data.communityCards.map(c => renderHoldemCard(c)).join('');
    }

    const playersEl = document.getElementById('holdem-players');
    const meSlotEl = document.getElementById('holdem-me-slot');
    if (playersEl && data.players) {
      const players = data.players;
      const numPlayers = 4;
      const myIdx = (players.find(p => p.userId === currentUserId)?.playerIdx ?? players.findIndex(p => p.userId === currentUserId)) % numPlayers;
      const RELATIVE_TO_SEAT = { 1: 'seat-left', 2: 'seat-top', 3: 'seat-right' };

      function renderPlayerBox(p, seatClass) {
        const isMe = p.userId === currentUserId;
        const isTurn = p.userId === data.currentTurn;
        const folded = p.status === 'fold';
        const cardsHtml = (p.cards || []).map(c => renderHoldemCard(c)).join('');
        const nameHtml = !isMe
          ? `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(p.userId)}')" title="전적 보기">${escapeHTML(p.userId)}</span>`
          : escapeHTML(p.userId) + ' (나)';
        const dealerBadge = p.isDealer ? '<div class="holdem-dealer-btn" title="딜러">D</div>' : '';
        const inner = `<div style="display:flex; align-items:center; gap:4px;">
            <div class="holdem-player-name" style="flex:1; min-width:0;">${nameHtml}</div>
            ${dealerBadge}
          </div>
          <div class="holdem-player-stars">⭐×${p.stars}</div>
          <div class="holdem-player-status">${folded ? '🏳️ 폴드' : p.status === 'check' ? '✅ 체크' : ''}</div>
          <div class="holdem-player-cards">${cardsHtml}</div>`;
        if (seatClass) {
          return `<div class="table-seat holdem-player-box ${seatClass} ${isTurn ? 'my-turn' : ''} ${folded ? 'folded' : ''}">${inner}</div>`;
        }
        return `<div class="holdem-player-box ${isTurn ? 'my-turn' : ''} ${folded ? 'folded' : ''}">${inner}</div>`;
      }

      const opponents = players
        .map(p => ({ ...p, relativeIdx: ((p.playerIdx ?? players.indexOf(p)) - myIdx + numPlayers) % numPlayers }))
        .filter(p => p.relativeIdx !== 0)
        .sort((a, b) => a.relativeIdx - b.relativeIdx);
      const me = players.find(p => p.userId === currentUserId);

      playersEl.innerHTML = opponents.map(p => renderPlayerBox(p, RELATIVE_TO_SEAT[p.relativeIdx] || 'seat-top')).join('');
      if (meSlotEl && me) {
        meSlotEl.innerHTML = renderPlayerBox(me, null);
      }
    }

    const canAct = isMyTurn && isPlayer && data.phase !== 'waiting' && data.phase !== 'showdown';
    document.getElementById('btn-holdem-check').disabled = !canAct;
    document.getElementById('btn-holdem-fold').disabled = !canAct;

    const holdemGuide = document.querySelector('#holdem-container .poker-hand-guide-grid');
    if (holdemGuide) {
      holdemGuide.querySelectorAll('.poker-hand-item').forEach(el => el.classList.remove('active'));
      const myHand = (data.myHandName || '').trim();
      if (myHand) {
        holdemGuide.querySelectorAll('.poker-hand-item').forEach(el => {
          const nameEl = el.querySelector('.poker-hand-name');
          if (nameEl && nameEl.textContent.trim() === myHand) el.classList.add('active');
        });
      }
    }
  }

  function holdemCheck() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'check' });
  }

  function holdemFold() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'fold' });
  }