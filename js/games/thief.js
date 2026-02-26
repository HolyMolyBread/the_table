  // ── Thief (도둑잡기) UI ────────────────────────────────────────────────────

  let thiefHoveredIndex = -1;
  let thiefHoveredTargetId = '';
  let lastThiefState = null;

  function showThiefUI() {
    switchGameView('thief');
    thiefHoveredIndex = -1;
    thiefHoveredTargetId = '';
  }

  function renderThiefCard(card, hoverFinger, isDiscarding) {
    if (!card) return '';
    const isRed = card.suit === '♥' || card.suit === '♦';
    const discardClass = isDiscarding ? ' discarding-pair' : '';
    const suit = card.suit || '🃏';
    const val = card.value || '?';
    const fingerClass = hoverFinger ? ' hover-finger' : '';
    return `<div class="thief-card ${isRed ? 'red-suit' : 'black-suit'}${fingerClass}${discardClass}"><span>${val}</span><span>${suit}</span></div>`;
  }

  // 시계방향 좌석 매핑: 상대 인덱스 1=왼쪽, 2=위, 3=오른쪽 (내 다음 사람이 왼쪽에 표시)
  const RELATIVE_INDEX_TO_SEAT = { 1: 'seat-left', 2: 'seat-top', 3: 'seat-right' };
  let lastThiefHandJson = '';

  function renderThief(data) {
    if (!data) return;
    lastThiefState = data;
    const isMyTurn = data.turn === currentUserId;
    document.getElementById('thief-status').textContent = isMyTurn
      ? '🎯 내 차례 — 상대방 카드를 클릭하여 뽑으세요!'
      : `⏳ ${escapeHTML(data.turn || '—')}의 차례`;
    document.getElementById('thief-escaped').textContent = data.escaped && data.escaped.length
      ? `탈출: ${data.escaped.join(', ')}`
      : '';

    const players = data.players || [];
    const numPlayers = players.length;
    const myIdx = players.findIndex(p => p.userId === currentUserId);
    const opponents = players
      .map((p, playerIdx) => ({ ...p, playerIdx }))
      .filter(p => p.userId !== currentUserId);
    const playersEl = document.getElementById('thief-players');
    if (playersEl) {
      playersEl.innerHTML = opponents.map((p) => {
        const relativeIdx = (p.playerIdx - myIdx + numPlayers) % numPlayers;
        const seatClass = opponents.length === 1 ? 'seat-top' : (RELATIVE_INDEX_TO_SEAT[relativeIdx] || 'seat-top');
        const isTarget = data.targetUserId === p.userId;
        const cardCount = p.cardCount || 0;
        let targetCardsHtml = '';
        if (isTarget && cardCount > 0) {
          targetCardsHtml = '<div class="table-seat-target-cards">' + Array.from({ length: cardCount }, (_, i) => {
            const hovered = thiefHoveredTargetId === p.userId && thiefHoveredIndex === i;
            return `<div class="thief-target-card${hovered ? ' hovered' : ''}" data-target-id="${escapeHTML(p.userId)}" data-index="${i}">🃏</div>`;
          }).join('') + '</div>';
        }
        const isTheirTurn = data.turn === p.userId;
        return `<div class="table-seat ${seatClass} ${isTheirTurn ? 'my-turn' : ''}" data-user-id="${escapeHTML(p.userId)}">
          <span class="table-seat-name">${escapeHTML(p.userId)}</span>
          <span class="table-seat-count">🃏 ${cardCount}장</span>
          ${targetCardsHtml}
        </div>`;
      }).join('');
      playersEl.querySelectorAll('.thief-target-card').forEach(el => {
        el.onclick = () => {
          const targetId = el.dataset.targetId;
          const index = parseInt(el.dataset.index, 10);
          thiefOnTargetCardClick(targetId, index, el);
        };
      });
    }

    const handEl = document.getElementById('thief-hand');
    if (handEl && data.hand) {
      const discardingSet = new Set((data.discardingPairs || []));
      const handJson = JSON.stringify(data.hand);
      if (handJson !== lastThiefHandJson) {
        lastThiefHandJson = handJson;
        const hoverOnMyCard = thiefHoveredTargetId === currentUserId;
        handEl.innerHTML = data.hand.map((c, i) => renderThiefCard(c, hoverOnMyCard && thiefHoveredIndex === i, discardingSet.has(i))).join('');
        handEl.querySelectorAll('.thief-card').forEach((el) => {
          if (window.applyCardFlipAnim) window.applyCardFlipAnim(el);
        });
      } else {
        const hoverOnMyCard = thiefHoveredTargetId === currentUserId;
        handEl.querySelectorAll('.thief-card').forEach((el, i) => {
          el.classList.toggle('hover-finger', hoverOnMyCard && thiefHoveredIndex === i);
          el.classList.toggle('discarding-pair', discardingSet.has(i));
        });
      }
    }

    setTimeout(() => {
      document.querySelectorAll('.table-seat').forEach(el => el.classList.remove('target-seat'));
      if (data.targetUserId) {
        document.querySelectorAll('.table-seat').forEach(el => {
          if (el.getAttribute('data-user-id') === data.targetUserId) {
            el.classList.add('target-seat');
          }
        });
      }
    }, 50);
  }

  function thiefOnTargetCardClick(targetId, index, el) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    if (thiefHoveredTargetId === targetId && thiefHoveredIndex === index) {
      if (window.isDrawing) return;
      window.isDrawing = true;
      setTimeout(() => { window.isDrawing = false; }, 500);
      sendGameAction({ cmd: 'draw', targetId: targetId, index: index });
      thiefHoveredIndex = -1;
      thiefHoveredTargetId = '';
    } else {
      sendGameAction({ cmd: 'hover', targetId: targetId, index: index });
      thiefHoveredIndex = index;
      thiefHoveredTargetId = targetId;
      document.querySelectorAll('#thief-players .thief-target-card').forEach(c => c.classList.remove('hovered'));
      if (el) el.classList.add('hovered');
    }
  }

  function thiefDraw() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'draw' });
  }