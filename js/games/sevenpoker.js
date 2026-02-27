  // ── Seven Poker UI ────────────────────────────────────────────────────────

  let spChoiceDiscard = -1;
  let spChoiceOpen = -1;
  let spChoiceMyCards = [];
  let lastSevenPokerMyTurn = false;

  function showSevenPokerUI() {
    switchGameView('sevenpoker');
  }

  function renderSevenPoker(data) {
    if (!data) return;

    const isMyTurn = data.currentTurn === currentUserId;
    const isPlayer = data.players && data.players.some(p => p.userId === currentUserId);
    if (isMyTurn && !lastSevenPokerMyTurn && window.SoundManager) {
      window.SoundManager.playPianoNote(783.99, 0.5);
    }
    lastSevenPokerMyTurn = isMyTurn;

    const roundBar = document.getElementById('sevenpoker-round-bar');
    const timerBlock = document.getElementById('sevenpoker-timer-block');
    if (roundBar) {
      roundBar.textContent = data.phase === 'choice' ? '모든 플레이어 선택 중' : `라운드 ${data.round || 0}`;
    }
    if (timerBlock) {
      const label = timerBlock.querySelector('.status-label');
      if (label) label.textContent = data.phase === 'choice' ? '모든 플레이어 선택 중' : '남은 시간';
    }
    document.getElementById('sevenpoker-pot-bar').textContent = `팟 ⭐×${data.pot || 0}`;

    const playersEl = document.getElementById('sevenpoker-players');
    const meSlotEl = document.getElementById('sevenpoker-me-slot');
    if (playersEl && data.players) {
      const players = data.players;
      const numPlayers = 4;
      const myIdx = (players.find(p => p.userId === currentUserId)?.playerIdx ?? players.findIndex(p => p.userId === currentUserId)) % numPlayers;
      const RELATIVE_TO_SEAT = { 1: 'seat-left', 2: 'seat-top', 3: 'seat-right' };

      function renderPlayerBox(p, seatClass) {
        const isMe = p.userId === currentUserId;
        const isTurn = p.userId === data.currentTurn;
        const folded = p.status === 'fold';
        const cardsHtml = (p.cards || []).map(c => {
          if (isMe && c.hidden) {
            let html = renderHoldemCard({ ...c, hidden: false });
            return html.replace('class="holdem-card', 'class="holdem-card my-secret-card');
          } else {
            return renderHoldemCard(c);
          }
        }).join('');
        const nameHtml = !isMe
          ? `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(p.userId)}')" title="전적 보기">${escapeHTML(p.userId)}</span>`
          : escapeHTML(p.userId) + ' (나)';
        const inner = `<div style="display:flex; align-items:center; gap:4px;"><div class="sevenpoker-player-name" style="flex:1; min-width:0;">${nameHtml}</div></div>
            <div class="sevenpoker-player-stars">⭐×${p.stars}</div>
            <div class="sevenpoker-player-status">${folded ? '🏳️ 폴드' : p.status === 'check' ? '✅ 체크' : ''}</div>
            <div class="sevenpoker-player-cards">${cardsHtml}</div>`;
        if (seatClass) {
          return `<div class="table-seat sevenpoker-player-box ${seatClass} ${isMe ? 'is-me' : 'is-opponent'} ${isTurn ? 'my-turn' : ''} ${folded ? 'folded' : ''}">${inner}</div>`;
        }
        return `<div class="sevenpoker-player-box is-me ${isTurn ? 'my-turn' : ''} ${folded ? 'folded' : ''}">${inner}</div>`;
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

    const canAct = isMyTurn && isPlayer && data.phase !== 'waiting' && data.phase !== 'showdown' && data.phase !== 'choice';
    document.getElementById('btn-sevenpoker-check').disabled = !canAct;
    document.getElementById('btn-sevenpoker-fold').disabled = !canAct;

    const choiceBox = document.getElementById('sevenpoker-choice-box');
    if (choiceBox) {
      const showChoice = data.phase === 'choice' && isPlayer && !data.myChoiceDone;
      choiceBox.style.display = showChoice ? 'block' : 'none';
      if (showChoice) {
        spChoiceDiscard = -1; spChoiceOpen = -1;
        const me = data.players.find(p => p.userId === currentUserId);
        if (me && me.cards) { spChoiceMyCards = me.cards; renderSpChoiceUI(); }
      }
    }

    const sevenGuide = document.querySelector('#sevenpoker-container .poker-hand-guide-grid');
    if (sevenGuide) {
      sevenGuide.querySelectorAll('.poker-hand-item').forEach(el => el.classList.remove('active'));
      const myHand = (data.myHandName || '').trim();
      if (myHand) {
        sevenGuide.querySelectorAll('.poker-hand-item').forEach(el => {
          const nameEl = el.querySelector('.poker-hand-name');
          if (nameEl && nameEl.textContent.trim() === myHand) el.classList.add('active');
        });
      }
    }
  }

  function sevenpokerCheck() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (window.SoundManager) window.SoundManager.playPianoNote(659.25, 0.4);
    sendGameAction({ cmd: 'check' });
  }

  function sevenpokerFold() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (window.SoundManager) window.SoundManager.playPianoNote(659.25, 0.4);
    sendGameAction({ cmd: 'fold' });
  }

  window.setSpChoice = function(type, idx) {
    if (type === 'discard') { spChoiceDiscard = idx; if (spChoiceOpen === idx) spChoiceOpen = -1; }
    if (type === 'open') { spChoiceOpen = idx; if (spChoiceDiscard === idx) spChoiceDiscard = -1; }
    renderSpChoiceUI();
  };
  window.renderSpChoiceUI = function() {
    const container = document.getElementById('sp-choice-cards');
    if (!container) return;
    container.innerHTML = spChoiceMyCards.map((c, i) => {
      const isDiscard = spChoiceDiscard === i;
      const isOpen = spChoiceOpen === i;
      const style = isDiscard ? 'opacity:0.4; border-color:var(--danger);' : (isOpen ? 'border-color:var(--accent); box-shadow:0 0 8px var(--accent);' : '');
      let cardHtml = renderHoldemCard({...c, hidden: false});
      if (style) cardHtml = cardHtml.replace(/^<div /, '<div style="' + style + '" ');
      return `
        <div style="display:flex; flex-direction:column; gap:6px; align-items:center;">
          ${cardHtml}
          <div style="display:flex; gap:4px;">
            <button onclick="setSpChoice('discard', ${i})" style="padding:4px 6px; font-size:11px; background:${isDiscard?'var(--danger)':'var(--bg-tertiary)'}; color:#fff; border:1px solid var(--border); border-radius:4px; cursor:pointer;">❌</button>
            <button onclick="setSpChoice('open', ${i})" style="padding:4px 6px; font-size:11px; background:${isOpen?'var(--accent)':'var(--bg-tertiary)'}; color:#fff; border:1px solid var(--border); border-radius:4px; cursor:pointer;">👁️</button>
          </div>
        </div>
      `;
    }).join('');
    const btn = document.getElementById('btn-sp-choice-submit');
    if (btn) btn.disabled = (spChoiceDiscard === -1 || spChoiceOpen === -1);
  };
  window.sendSevenPokerChoice = function() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (spChoiceDiscard === -1 || spChoiceOpen === -1) return;
    if (window.SoundManager) window.SoundManager.playPianoNote(659.25, 0.4);
    sendGameAction({ cmd: 'choice', discardIdx: spChoiceDiscard, openIdx: spChoiceOpen });
    spChoiceDiscard = -1; spChoiceOpen = -1;
  };

  /** 포커(홀덤/세븐포커) 쇼다운 결과 오버레이 — 5초간 표시 후 자동 닫힘 */
  let pokerShowdownTimeout = null;
  function showPokerShowdownOverlay(data) {
    if (!data) return;
    const winnerEl = document.getElementById('poker-showdown-winner');
    const partEl = document.getElementById('poker-showdown-participants');
    const overlay = document.getElementById('poker-showdown-overlay');

    const winnerId = data.winnerId || '';
    const winningHand = data.winningHand || '';
    const participants = data.participants || [];
    winnerEl.textContent = winningHand ? `승자: ${winnerId} (${winningHand})` : `승자: ${winnerId}`;

    let html = '';
    participants.forEach(p => {
      const delta = p.deltaStars ?? 0;
      let deltaHtml = '';
      if (delta > 0) {
        deltaHtml = ` <span style="color:var(--success);">(+${delta})</span>`;
      } else if (delta < 0) {
        deltaHtml = ` <span style="color:var(--danger);">(${delta})</span>`;
      }
      html += `<div class="poker-showdown-row"><span>${escapeHTML(p.userId || '')}${deltaHtml}</span><span>${escapeHTML(p.handName || '-')}</span></div>`;
    });
    partEl.innerHTML = html || '<div class="poker-showdown-row">—</div>';

    if (pokerShowdownTimeout) clearTimeout(pokerShowdownTimeout);
    overlay.classList.add('show');
    pokerShowdownTimeout = setTimeout(() => {
      overlay.classList.remove('show');
      pokerShowdownTimeout = null;
    }, 5000);
  }

  function closePokerShowdownOverlay() {
    const overlay = document.getElementById('poker-showdown-overlay');
    if (overlay) overlay.classList.remove('show');
    if (pokerShowdownTimeout) {
      clearTimeout(pokerShowdownTimeout);
      pokerShowdownTimeout = null;
    }
  }