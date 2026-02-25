  // ── OneCard (원카드) UI ────────────────────────────────────────────────────

  function showOneCardUI() {
    switchGameView('onecard');
  }

  function renderOneCardCard(card, playable) {
    if (!card) return '';
    const isRed = card.suit === '♥' || card.suit === '♦';
    const suit = card.suit || '🃏';
    let val = card.value || '?';
    if (val === 'B_JOKER') val = 'B';
    if (val === 'C_JOKER') val = 'C';
    return `<div class="onecard-card ${isRed ? 'red-suit' : 'black-suit'} ${playable ? 'playable' : ''}" data-index="${card._index ?? ''}"><span>${val}</span><span>${suit}</span></div>`;
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
    const suit = data.targetSuit || top.suit || '';
    const attackPenalty = data.attackPenalty || 0;
    if (attackPenalty > 0) return onecardCanDefend(top, card);
    const cv = card.value || '';
    if (cv === 'B_JOKER' || cv === 'C_JOKER') return true;
    return (card.suit === suit || card.value === top.value);
  }

  let onecardPendingPlayIndex = -1;

  function renderOneCard(data) {
    if (!data) return;
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

    const playersEl = document.getElementById('onecard-players');
    if (playersEl && data.players) {
      const opponents = (data.players || []).filter(p => p.userId !== currentUserId);
      playersEl.innerHTML = opponents.map((p, idx) => {
        const seatClass = TABLE_SEAT_ORDER[idx] || 'seat-top';
        return `<div class="table-seat onecard-player-box ${seatClass} ${p.isTurn ? 'my-turn' : ''}" data-user-id="${escapeHTML(p.userId)}">
          <span class="table-seat-name">${escapeHTML(p.userId)}</span>
          <span class="table-seat-count">🃏 ${p.cardCount || 0}장</span>
        </div>`;
      }).join('');
    }

    const topEl = document.getElementById('onecard-top-card');
    if (topEl) topEl.innerHTML = top.suit ? renderOneCardCard(top, false).replace(' data-index=""', '') : '';
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
      const hand = data.hand ?? [];
      handEl.innerHTML = hand.map((c, i) => {
        const playable = canPlay && onecardIsPlayable(data, c);
        const cardWithIdx = { ...c, _index: i };
        return renderOneCardCard(cardWithIdx, playable);
      }).join('');
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