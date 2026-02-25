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
    if (playersEl && data.players) {
      playersEl.innerHTML = data.players.map(p => {
        const isMe = p.userId === currentUserId;
        const isTurn = p.userId === data.currentTurn;
        const folded = p.status === 'fold';
        const cardsHtml = (p.cards || []).map(c => renderHoldemCard(c)).join('');
        const nameHtml = !isMe
          ? `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(p.userId)}')" title="전적 보기">${escapeHTML(p.userId)}</span>`
          : escapeHTML(p.userId) + ' (나)';
        return `
          <div class="holdem-player-box ${isTurn ? 'my-turn' : ''} ${folded ? 'folded' : ''}">
            <div style="display:flex; justify-content:space-between; align-items:center; gap:4px;">
              <div class="holdem-player-name" style="flex:1;">${nameHtml}</div>
              ${p.userId === data.dealerId ? `<div class="holdem-dealer-btn" title="딜러">D</div>` : ''}
            </div>
            <div class="holdem-player-stars">⭐×${p.stars}</div>
            <div class="holdem-player-status">${folded ? '🏳️ 폴드' : p.status === 'check' ? '✅ 체크' : ''}</div>
            <div class="holdem-player-cards">${cardsHtml}</div>
          </div>`;
      }).join('');
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