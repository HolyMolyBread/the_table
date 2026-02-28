  // ── Blackjack UI ───────────────────────────────────────────────────────────
  let lastBlackjackMyTurn = false;

  function showBlackjackUI() {
    switchGameView('blackjack');
  }

  /** 블랙잭 하트 표시: 숫자 형식 (❤️ x N) */
  function renderBJHeartsBar(count) {
    const n = Math.max(0, count ?? 0);
    return `❤️ x ${n}`;
  }

  function renderBJPlayerBox(playerData, isMe, isTheirTurn, showActions, isReady) {
    const handInfo = playerData?.hand ? { cards: playerData.hand, score: handScoreFromCards(playerData.hand) } : { cards: [], score: 0 };
    const hearts = playerData?.hearts ?? 0;
    const name = isMe ? '나' : (playerData?.userId ? escapeHTML(playerData.userId) : '—');
    const ready = isReady === true;
    const readyBtnText = ready ? '✓ 준비 완료' : '준비';
    const readyBtnDisabled = ready ? ' disabled' : '';
    const readyBtnStyle = ready ? ' style="display:none"' : '';
    const cardsHtml = (handInfo.cards || []).map(card => {
      if (card.hidden) return `<div class="bj-card hidden"></div>`;
      const suit = card.suit || card.Suit || '';
      const value = card.value || card.Value || '?';
      const isRed = suit === '♥' || suit === '♦';
      return `<div class="bj-card ${isRed ? 'red' : 'black'}">
        <span class="bj-card-top">${value}</span>
        <span class="bj-card-center">${suit}</span>
        <span class="bj-card-bot">${value}</span>
      </div>`;
    }).join('');
    const heartsText = renderBJHeartsBar(hearts);
    const actionsHtml = showActions ? `
      <div class="bj-player-actions">
        <div id="bj-start-buttons">
          <button class="bj-btn bj-btn-bet" id="bj-ready-btn" onclick="bjStart()"${readyBtnDisabled}${readyBtnStyle}>${readyBtnText}</button>
        </div>
        <div id="bj-game-buttons" style="display:none">
          <button class="bj-btn bj-btn-hit" onclick="bjHit()">Hit</button>
          <button class="bj-btn bj-btn-stand" onclick="bjStand()">Stand</button>
        </div>
      </div>
    ` : '';
    return `<div class="bj-player-box ${isMe ? 'is-me' : ''} ${isTheirTurn ? 'my-turn' : ''}">
      <div class="bj-area-header">
        <span class="bj-area-title">${isMe ? '👤' : '👤'} ${name}</span>
        <span class="bj-hearts-simple">${heartsText}</span>
      </div>
      <div class="bj-hand">${cardsHtml}</div>
      <div class="bj-score">${handInfo.score > 0 ? handInfo.score : ''}</div>
      ${actionsHtml}
    </div>`;
  }

  function renderBlackjackState(data) {
    if (!data) return;
    let isMyTurn = false;
    const players = data.Players || data.players;
    const turnOrder = data.turnOrder || data.TurnOrder;
    const currentTurnIdx = data.currentTurnIdx ?? data.CurrentTurnIdx ?? 0;
    if (players && turnOrder) {
      isMyTurn = data.phase === 'player_turn' && turnOrder[currentTurnIdx] === currentUserId;
    } else {
      isMyTurn = data.phase === 'player_turn';
    }
    if (isMyTurn && !lastBlackjackMyTurn && window.SoundManager) {
      window.SoundManager.playPianoNote(783.99, 0.5);
    }
    lastBlackjackMyTurn = isMyTurn;
    const dealerHand = data.DealerHand || data.dealerHand || [];
    const dealerHandInfo = Array.isArray(dealerHand) ? { cards: dealerHand, score: handScoreFromCards(dealerHand) } : dealerHand;
    renderBJHand('bj-dealer-hand', 'bj-dealer-score', dealerHandInfo);
    const dealerHeartsEl = document.getElementById('bj-dealer-hearts');
    if (dealerHeartsEl) dealerHeartsEl.innerHTML = renderBJHeartsBar(data.DealerHearts ?? data.dealerHearts ?? 10);

    const playersRowEl = document.getElementById('bj-players-row');
    if (!playersRowEl) return;

    if (players) {
      const turnOrder = data.turnOrder || data.TurnOrder || Object.keys(players);
      const totalPlayers = turnOrder.length;
      const myIdx = turnOrder.indexOf(currentUserId);
      const currentTurnIdx = data.currentTurnIdx ?? data.CurrentTurnIdx ?? 0;
      let seatMap;
      if (totalPlayers === 2) {
        seatMap = { top: (myIdx + 1) % 2, left: -1, right: -1 };
      } else if (totalPlayers === 3) {
        seatMap = { right: (myIdx + 1) % 3, top: (myIdx + 2) % 3, left: -1 };
      } else {
        const n = totalPlayers;
        seatMap = { right: (myIdx + 1) % n, top: (myIdx + 2) % n, left: (myIdx + 3) % n };
      }
      const order = [];
      if (seatMap.right >= 0) order.push(seatMap.right);
      if (seatMap.top >= 0) order.push(seatMap.top);
      order.push(myIdx);
      if (seatMap.left >= 0) order.push(seatMap.left);
      const ordered = order.map(i => ({ userId: turnOrder[i], playerIdx: i }));
      const readyStatus = data.ReadyStatus || data.readyStatus || {};
      playersRowEl.innerHTML = ordered.map(({ userId }) => {
        const p = players[userId];
        const hand = p ? (p.Hand || p.hand || []) : [];
        const hearts = p ? (p.Hearts ?? p.hearts ?? 0) : 0;
        const isMe = userId === currentUserId;
        const isTheirTurn = data.phase === 'player_turn' && turnOrder[currentTurnIdx] === userId;
        const showActions = isMe;
        const isReady = !!readyStatus[userId];
        return renderBJPlayerBox({ userId, hand, hearts }, isMe, isTheirTurn, showActions, isReady);
      }).join('');
    } else {
      const meData = { hand: data.playerHand?.cards || [], hearts: data.playerHearts ?? 0 };
      const readyStatus = data.ReadyStatus || data.readyStatus || {};
      const isReady = !!readyStatus[currentUserId];
      playersRowEl.innerHTML = renderBJPlayerBox(meData, true, isMyTurn, true, isReady);
    }

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

      const raidDetailsEl = document.getElementById('bj-raid-details');
      if (raidDetailsEl && data.raidResult) {
        const rr = data.raidResult;
        let html = '';
        if (rr.dealerDamage != null && rr.dealerDamage > 0) {
          html += `<div style="margin-bottom:8px;">👿 딜러 피해: -${rr.dealerDamage}HP</div>`;
        }
        if (rr.dealerHp != null) {
          const dealerHeartsEl = document.getElementById('bj-dealer-hearts');
          if (dealerHeartsEl) dealerHeartsEl.innerHTML = renderBJHeartsBar(rr.dealerHp);
        }
        if (rr.playerChanges && typeof rr.playerChanges === 'object') {
          Object.entries(rr.playerChanges).forEach(([userId, diff]) => {
            const label = userId === currentUserId ? '나' : escapeHTML(userId);
            const sign = diff >= 0 ? '+' : '';
            html += `<div style="margin-bottom:4px;">👤 ${label}: ${sign}${diff}HP</div>`;
          });
        }
        raidDetailsEl.innerHTML = html;
        raidDetailsEl.style.display = html ? 'block' : 'none';
      } else if (raidDetailsEl) {
        raidDetailsEl.innerHTML = '';
        raidDetailsEl.style.display = 'none';
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

    const isSpectator = data.mainPlayerId && data.mainPlayerId !== currentUserId;
    if (spectatorMsg) spectatorMsg.style.display = isSpectator ? 'block' : 'none';
    if (startBtns && gameBtns && !isSpectator) {
      if (data.phase === 'game_over') {
        startBtns.style.display = 'none';
        gameBtns.style.display = 'none';
      } else if (data.phase === 'betting' || data.phase === 'settlement') {
        startBtns.style.display = 'flex';
        gameBtns.style.display = 'none';
        const rematchArea = document.getElementById('rematch-area');
        if (rematchArea) { rematchArea.style.display = 'none'; rematchArea.classList.remove('visible'); }
      } else if (data.phase === 'player_turn') {
        startBtns.style.display = 'none';
        gameBtns.style.display = 'flex';
      } else {
        startBtns.style.display = 'none';
        gameBtns.style.display = 'none';
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
      const suit = card.suit || card.Suit || '';
      const value = card.value || card.Value || '?';
      const isRed = suit === '♥' || suit === '♦';
      return `<div class="bj-card ${isRed ? 'red' : 'black'}">
        <span class="bj-card-top">${value}</span>
        <span class="bj-card-center">${suit}</span>
        <span class="bj-card-bot">${value}</span>
      </div>`;
    }).join('');
    const score = handInfo.score;
    scoreEl.textContent = score > 0 ? `${score}` : '';
    scoreEl.className   = score > 21 ? 'bj-score bust' : 'bj-score';
  }

  function bjStart() {
    if (!ws || ws.readyState !== WebSocket.OPEN || !currentRoomId) return;
    sendGameAction({ cmd: 'ready' });
    const btn = document.getElementById('bj-ready-btn');
    if (btn) btn.style.display = 'none';
  }
  function bjHit() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (window.SoundManager) window.SoundManager.playPianoNote(523.25, 0.3);
    sendGameAction({ cmd: 'hit' });
  }
  function bjStand() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (window.SoundManager) window.SoundManager.playPianoNote(659.25, 0.4);
    sendGameAction({ cmd: 'stand' });
  }

  /** PVP 데이터를 PVE 형식으로 변환 (서버 JSON 필드명: Players, DealerHand) */
  function adaptPVPToPVEData(data) {
    const players = data?.Players || data?.players;
    if (!data || !players) return data;
    const dealerHand = data.DealerHand || data.dealerHand || [];
    const me = players[currentUserId];
    const handInfo = me ? { cards: me.Hand || me.hand || [], score: (me.Hand || me.hand) ? handScoreFromCards(me.Hand || me.hand) : 0 } : { cards: [], score: 0 };
    const turnOrder = data.turnOrder || data.TurnOrder;
    const isMyTurn = turnOrder && turnOrder[data.currentTurnIdx ?? data.CurrentTurnIdx] === currentUserId;
    return {
      phase: data.phase,
      playerHand: handInfo,
      dealerHand: { cards: dealerHand, score: handScoreFromCards(dealerHand) },
      playerHearts: me ? (me.Hearts ?? me.hearts) : 0,
      dealerHearts: data.DealerHearts ?? data.dealerHearts ?? 0,
      message: data.message,
      mainPlayerId: currentUserId,
      gameOverPlayerWin: data.gameOverWin,
      raidResult: data.raidResult,
      _isMyTurn: isMyTurn,
      _turnOrder: turnOrder,
      _currentTurnIdx: data.currentTurnIdx ?? data.CurrentTurnIdx,
    };
  }
  function handScoreFromCards(cards) {
    let total = 0, aces = 0;
    for (const c of cards || []) {
      if (c.hidden) continue;
      const v = c.value || c.Value || '';
      if (v === 'A') { total += 11; aces++; }
      else if (['J','Q','K'].includes(v)) total += 10;
      else total += parseInt(v, 10) || 0;
    }
    while (total > 21 && aces > 0) { total -= 10; aces--; }
    return total;
  }

  function renderBlackjackPVPState(data) {
    renderBlackjackState(data);
    const adapted = adaptPVPToPVEData(data);
    const gameBtns = document.getElementById('bj-game-buttons');
    const startBtns = document.getElementById('bj-start-buttons');
    if (gameBtns && startBtns && adapted) {
      const isMyTurn = adapted._isMyTurn && adapted.phase === 'player_turn';
      if (adapted.phase === 'player_turn') {
        startBtns.style.display = 'none';
        gameBtns.style.display = isMyTurn ? 'flex' : 'none';
      }
    }
  }

  function showBlackjackRaidResultOverlay(gameResultData) {
    const overlayEl = document.getElementById('bj-result-overlay');
    const boxEl = document.getElementById('bj-result-box');
    const msgTextEl = document.getElementById('bj-result-msg');
    const raidDetailsEl = document.getElementById('bj-raid-details');
    if (!overlayEl || !boxEl || !msgTextEl) return;
    msgTextEl.textContent = gameResultData.message || '게임 종료';
    boxEl.className = 'unified-result-box';
    boxEl.classList.toggle('win', gameResultData.data?.playerWin === true);
    boxEl.classList.toggle('lose', gameResultData.data?.playerWin === false);
    if (raidDetailsEl && gameResultData.data) {
      const d = gameResultData.data;
      let html = '';
      if (d.dealerDamage != null && d.dealerDamage > 0) {
        html += `<div style="margin-bottom:8px;">👿 딜러 피해: -${d.dealerDamage}HP</div>`;
      }
      if (d.dealerHp != null) {
        const dealerHeartsEl = document.getElementById('bj-dealer-hearts');
        if (dealerHeartsEl) dealerHeartsEl.innerHTML = renderBJHeartsBar(d.dealerHp);
      }
      if (d.playerChanges && typeof d.playerChanges === 'object') {
        Object.entries(d.playerChanges).forEach(([userId, diff]) => {
          const label = userId === currentUserId ? '나' : escapeHTML(userId);
          const sign = diff >= 0 ? '+' : '';
          html += `<div style="margin-bottom:4px;">👤 ${label}: ${sign}${diff}HP</div>`;
        });
      }
      raidDetailsEl.innerHTML = html;
      raidDetailsEl.style.display = html ? 'block' : 'none';
    }
    overlayEl.style.display = 'flex';
    if (window.bjResultTimer) clearTimeout(window.bjResultTimer);
    window.bjResultTimer = setTimeout(() => {
      overlayEl.style.display = 'none';
      window.bjResultTimer = null;
      if (typeof window.showReadyLayerForGameEnd === 'function') {
        const total = gameResultData.data?.totalCount ?? 2;
        window.showReadyLayerForGameEnd(total);
      }
    }, 5000);
  }

  window.renderBlackjackPVPState = renderBlackjackPVPState;
  window.adaptPVPToPVEData = adaptPVPToPVEData;
  window.showBlackjackRaidResultOverlay = showBlackjackRaidResultOverlay;