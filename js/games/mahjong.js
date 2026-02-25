  // ── Mahjong (마작) UI ───────────────────────────────────────────────────────

  function showMahjongUI() {
    switchGameView('mahjong');
  }

  function getMahjongTileHTML(type, value, isHidden = false) {
    if (isHidden) return '<div class="mahjong-tile hidden">🀫</div>';
    const dict = {
      'man': ['','🀇','🀈','🀉','🀊','🀋','🀌','🀍','🀎','🀏'],
      'pin': ['','🀙','🀚','🀛','🀜','🀝','🀞','🀟','🀠','🀡'],
      'sou': ['','🀐','🀑','🀒','🀓','🀔','🀕','🀖','🀗','🀘'],
      'honor': ['','🀀','🀁','🀂','🀃','🀆','🀅','🀄']
    };
    const char = dict[type] ? dict[type][value] : '🀫';
    return `<div class="mahjong-tile">${char}</div>`;
  }

  function mahjongTileChar(tile) {
    if (!tile || !tile.type) return '🀫';
    const man = ['', '🀇','🀈','🀉','🀊','🀋','🀌','🀍','🀎','🀏'];
    const pin = ['', '🀙','🀚','🀛','🀜','🀝','🀞','🀟','🀠','🀡'];
    const sou = ['', '🀐','🀑','🀒','🀓','🀔','🀕','🀖','🀗','🀘'];
    const honor = ['', '🀀','🀁','🀂','🀃','🀄','🀅','🀆'];
    const v = tile.value || 0;
    if (tile.type === 'man' && v >= 1 && v <= 9) return man[v];
    if (tile.type === 'pin' && v >= 1 && v <= 9) return pin[v];
    if (tile.type === 'sou' && v >= 1 && v <= 9) return sou[v];
    if (tile.type === 'honor' && v >= 1 && v <= 7) return honor[v];
    return '🀫';
  }

  function renderMahjong(data) {
    if (!data) return;
    const isMyTurn = data.currentTurn === currentUserId;
    const myHand = data.myHand || [];
    const canDiscard = isMyTurn && myHand.length === 14;

    document.getElementById('mahjong-status').textContent = isMyTurn
      ? '🎯 내 차례 — 패를 클릭해 버리세요!'
      : `⏳ ${escapeHTML(data.currentTurn || '—')}의 차례`;
    document.getElementById('mahjong-wall-info').textContent = `🀄 남은 패: ${data.wallCount ?? 0}장`;

    const playersEl = document.getElementById('mahjong-players');
    if (playersEl && data.players) {
      const myIdx = data.players.findIndex(p => p && p.userId === currentUserId);
      const opponentIndices = myIdx >= 0 ? [(myIdx + 2) % 4, (myIdx + 3) % 4, (myIdx + 1) % 4] : [0, 1, 2];
      const opponents = opponentIndices.map(i => data.players[i]).filter(p => p && p.userId);
      playersEl.innerHTML = opponents.map((p, idx) => {
        const seatClass = TABLE_SEAT_ORDER[idx] || 'seat-top';
        const discardsHtml = (p.discards || []).map(t => getMahjongTileHTML(t.type || t.Type, t.value ?? t.Value ?? 0, false)).join('');
        const handHtml = Array(p.handCount || 0).fill(0).map(() => getMahjongTileHTML('', 0, true)).join('');
        return `<div class="table-seat mahjong-player-box ${seatClass} ${p.isTurn ? 'my-turn' : ''}" data-user-id="${escapeHTML(p.userId)}">
          <span class="table-seat-name">${escapeHTML(p.userId)}</span>
          <div class="mahjong-discards mahjong-discards-row">${discardsHtml}</div>
          <div class="mahjong-hand opponent-hand">${handHtml}</div>
        </div>`;
      }).join('');
    }

    const discardsMeEl = document.getElementById('mahjong-discards-me');
    const mePlayer = data.players?.find(p => p.userId === currentUserId);
    if (discardsMeEl && mePlayer) {
      discardsMeEl.innerHTML = (mePlayer.discards || []).map(t => getMahjongTileHTML(t.type || t.Type, t.value ?? t.Value ?? 0, false)).join('');
    }

    const handEl = document.getElementById('mahjong-hand');
    if (handEl && myHand) {
      handEl.innerHTML = myHand.map((t, i) => {
        const discardable = canDiscard ? ' discardable' : '';
        const type = t.type || t.Type;
        const value = t.value ?? t.Value ?? 0;
        return `<div class="mahjong-tile${discardable}" data-index="${i}">${getMahjongTileHTML(type, value, false).replace(/^<div[^>]*>|<\/div>$/g, '')}</div>`;
      }).join('');
      if (canDiscard) {
        handEl.querySelectorAll('.mahjong-tile.discardable').forEach(el => {
          el.style.cursor = 'pointer';
          el.onclick = () => {
            const idx = parseInt(el.dataset.index, 10);
            if (!isNaN(idx)) mahjongDiscard(idx);
          };
        });
      }
    }
  }

  function mahjongDiscard(index) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'discard', index });
  }