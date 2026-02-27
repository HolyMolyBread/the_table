  // ── OneCard (원카드) UI ────────────────────────────────────────────────────

  let lastOneCardTopJson = '';
  let lastOneCardHandJson = '';

  function showOneCardUI() {
    switchGameView('onecard');
    lastOneCardTopJson = '';
    lastOneCardHandJson = '';
  }

  function renderOneCardCard(card, playable) {
    if (!card) return '';
    const isRed = card.suit === '♥' || card.suit === '♦';
    const isRedJoker = card.value === 'C_JOKER';
    const suit = card.suit || '🃏';
    let val = card.value || '?';
    if (val === 'B_JOKER') val = 'B';
    if (val === 'C_JOKER') val = 'C';

    return `
    <div class="onecard-card ${isRed ? 'red-suit' : 'black-suit'} ${isRedJoker ? 'red-joker' : ''} ${playable ? 'playable' : ''}" data-index="${card._index ?? ''}">
      <div style="font-size:12px; font-weight: bold; text-align:left;">${val}</div>
      <div style="font-size:24px; text-align:center; flex: 1; display: flex; align-items: center; justify-content: center;">${suit}</div>
      <div style="font-size:12px; font-weight: bold; text-align:right;">${val}</div>
    </div>`;
  }

  function onecardCanDefend(attackCard, card) {
    if (!attackCard || !card) return false;
    const av = attackCard.value || '';
    const cv = card.value || '';
    if (av === 'A') return cv === 'A' || cv === 'B_JOKER' || cv === 'C_JOKER';
    if (av === 'B_JOKER') return cv === 'C_JOKER';
    if (av === 'C_JOKER') return false;
    return false;
  }
  function onecardIsPlayable(data, card) {
    const top = data.topCard || {};
    const attackPenalty = data.attackPenalty || 0;
    if (attackPenalty > 0) return onecardCanDefend(top, card);
    const cv = card.value || '';
    if (cv === 'B_JOKER' || cv === 'C_JOKER') return true;
    // targetSuit가 있으면 해당 문양으로 낼 수 있음 (조커 정산 후 등)
    const targetSuit = data.targetSuit || '';
    if (targetSuit && card.suit === targetSuit) return true;
    // targetSuit 없으면 topCard 기준
    const suit = targetSuit || top.suit || '';
    if (suit && card.suit === suit) return true;
    if (top.value && card.value === top.value) return true;
    // 조커 위: B_JOKER(흑백) → ♠♣, C_JOKER(컬러) → ♥♦
    if (top.value === 'B_JOKER') return card.suit === '♠' || card.suit === '♣';
    if (top.value === 'C_JOKER') return card.suit === '♥' || card.suit === '♦';
    return false;
  }

  let onecardPendingPlayIndex = -1;

  function renderOneCard(data) {
    if (!data) return;
    console.log("[OneCard Debug]", data, currentUserId);
    const isMyTurn = data.turn === currentUserId;
    const top = data.topCard || {};
    const attackPenalty = data.attackPenalty || 0;
    const oneCardVuln = data.oneCardVulnerable || '';

    document.getElementById('onecard-status').textContent = isMyTurn
      ? (attackPenalty > 0 ? '🛡️ 방어 카드를 내거나 드로우하세요!' : '🎯 내 차례 — 카드를 내거나 드로우하세요!')
      : `⏳ ${escapeHTML(data.turn || '—')}의 차례`;
    document.getElementById('onecard-direction').textContent = (data.direction === -1 ? '↺ 반시계' : '↻ 시계') + ' 방향';

    const attackBanner = document.getElementById('onecard-attack-banner');
    const attackCount = document.getElementById('onecard-attack-count');
    if (attackBanner && attackCount) {
      attackCount.textContent = attackPenalty;
      attackBanner.classList.toggle('show', attackPenalty > 0);
    }

    const btnCall = document.getElementById('btn-onecard-call');
    if (btnCall) {
      btnCall.disabled = !oneCardVuln || !ws || ws.readyState !== WebSocket.OPEN;
    }

    const meStatusEl = document.getElementById('onecard-me-status');
    if (meStatusEl && data.players) {
      const mePlayer = data.players.find(p => p.userId === currentUserId);
      if (mePlayer) {
        if (mePlayer.status === 'escaped') {
          meStatusEl.textContent = '탈출 성공';
          meStatusEl.style.display = '';
        } else if (mePlayer.status === 'bankrupt') {
          meStatusEl.textContent = '파산';
          meStatusEl.style.display = '';
        } else {
          meStatusEl.style.display = 'none';
        }
      } else {
        meStatusEl.style.display = 'none';
      }
    }

    const playersEl = document.getElementById('onecard-players');
    if (playersEl && data.players) {
      const players = data.players;
      const numPlayers = players.length;
      const myIdx = players.findIndex(p => p.userId === currentUserId);
      const RELATIVE_INDEX_TO_SEAT = { 1: 'seat-left', 2: 'seat-top', 3: 'seat-right' };
      const opponents = players
        .map((p, playerIdx) => ({ ...p, playerIdx, relativeIdx: (playerIdx - myIdx + numPlayers) % numPlayers }))
        .filter(p => p.userId !== currentUserId)
        .sort((a, b) => a.relativeIdx - b.relativeIdx);
      playersEl.innerHTML = opponents.map((p) => {
        const seatClass = opponents.length === 1 ? 'seat-left' : (RELATIVE_INDEX_TO_SEAT[p.relativeIdx] || 'seat-top');
        let countText = `🃏 ${p.cardCount || 0}장`;
        if (p.status === 'escaped') countText = '탈출 성공';
        else if (p.status === 'bankrupt') countText = '파산';
        return `<div class="table-seat onecard-player-box ${seatClass} ${p.isTurn ? 'my-turn' : ''}" data-user-id="${escapeHTML(p.userId)}">
          <span class="table-seat-name">${escapeHTML(p.userId)}</span>
          <span class="table-seat-count">${countText}</span>
        </div>`;
      }).join('');
    }

    const topEl = document.getElementById('onecard-top-card');
    if (topEl) {
      const topJson = JSON.stringify(top);
      if (topJson !== lastOneCardTopJson) {
        lastOneCardTopJson = topJson;
        topEl.innerHTML = top.suit ? renderOneCardCard(top, false).replace(' data-index=""', '') : '';
        const cardEl = topEl.firstElementChild || topEl;
        if (cardEl && top.suit && window.applyCardFlipAnim) window.applyCardFlipAnim(cardEl);
      }
    }
    const canPlay = isMyTurn && top.suit;
    const hasPlayableCard = canPlay && (data.hand?.some(c => onecardIsPlayable(data, c)) ?? false);
    const deckEl = document.getElementById('onecard-deck');
    if (deckEl) {
      const total = (data.deckCount || 0) + (data.discardCount || 0);
      deckEl.textContent = total > 0 ? `🃏 ${data.deckCount || 0}` : '';
      deckEl.style.cursor = isMyTurn && total > 0 ? 'pointer' : 'default';
      deckEl.onclick = (total > 0 && isMyTurn) ? onecardDraw : null;
      if (isMyTurn && !hasPlayableCard && total > 0) {
        deckEl.classList.add('highlight-deck');
      } else {
        deckEl.classList.remove('highlight-deck');
      }
    }
    const handEl = document.getElementById('onecard-hand');
    if (handEl) {
      const hand = data.hand || [];
      if (hand.length === 0 && data.players && data.players.some(p => p.userId === currentUserId)) {
        console.warn("내 손패가 비어있습니다. 서버 데이터를 점검하세요.");
      }
      const handJson = JSON.stringify(hand.map((c, i) => ({ ...c, _playable: canPlay && onecardIsPlayable(data, c), _index: i })));
      if (handJson !== lastOneCardHandJson) {
        lastOneCardHandJson = handJson;
        handEl.innerHTML = hand.map((c, i) => {
          const playable = canPlay && onecardIsPlayable(data, c);
          const cardWithIdx = { ...c, _index: i };
          return renderOneCardCard(cardWithIdx, playable);
        }).join('');
        handEl.querySelectorAll('.onecard-card').forEach((el) => {
          if (window.applyCardFlipAnim) window.applyCardFlipAnim(el);
        });
        handEl.querySelectorAll('.onecard-card.playable').forEach(el => {
          el.style.cursor = 'pointer';
          el.onclick = () => {
            const idx = parseInt(el.dataset.index, 10);
            if (isNaN(idx)) return;
            const card = hand[idx];
            if (card && card.value === '7') {
              onecardPendingPlayIndex = idx;
              document.getElementById('onecard-suit-modal').classList.add('show');
            } else {
              onecardPlay(idx);
            }
          };
        });
      } else {
        handEl.querySelectorAll('.onecard-card').forEach((el, i) => {
          const playable = canPlay && onecardIsPlayable(data, hand[i]);
          el.classList.toggle('playable', playable);
          el.dataset.index = String(i);
          if (playable) {
            el.style.cursor = 'pointer';
            el.onclick = () => {
              const idx = parseInt(el.dataset.index, 10);
              if (isNaN(idx)) return;
              const card = hand[idx];
              if (card && card.value === '7') {
                onecardPendingPlayIndex = idx;
                document.getElementById('onecard-suit-modal').classList.add('show');
              } else {
                onecardPlay(idx);
              }
            };
          } else {
            el.onclick = null;
          }
        });
      }
    }
  }

  function closeOneCardSuitModal() {
    document.getElementById('onecard-suit-modal').classList.remove('show');
    onecardPendingPlayIndex = -1;
  }
  function onecardPickSuit(suit) {
    if (onecardPendingPlayIndex < 0) return;
    onecardPlay(onecardPendingPlayIndex, suit);
    closeOneCardSuitModal();
  }

  function onecardDraw() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'draw' });
  }

  function onecardPlay(index, targetSuit) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    const payload = { cmd: 'play', index };
    if (targetSuit) payload.targetSuit = targetSuit;
    sendGameAction(payload);
  }

  function onecardCallOneCard() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'call_onecard' });
  }