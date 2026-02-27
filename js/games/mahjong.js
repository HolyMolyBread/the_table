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

    const statusEl = document.getElementById('mahjong-status');
    if (statusEl) {
      if (data.callWindow) {
        statusEl.textContent = data.lastDiscarderId === currentUserId
          ? '⏸ 콜 대기 중 — 다른 플레이어가 치/퐁/패스를 선택합니다'
          : '🔔 콜 대기 — 치, 퐁, 또는 패스를 선택하세요!';
      } else {
        statusEl.textContent = isMyTurn
          ? '🎯 내 차례 — 패를 클릭해 버리세요!'
          : `⏳ ${escapeHTML(data.currentTurn || '—')}의 차례`;
      }
    }
    const wallInfo = document.getElementById('mahjong-wall-info');
    if (wallInfo) wallInfo.textContent = `🀄 남은 패: ${data.wallCount ?? 0}장`;

    // center-pond: 모든 플레이어 버림패 6개씩 줄바꿈
    const centerPond = document.getElementById('mahjong-center-pond');
    if (centerPond) {
      const allDiscards = [];
      (data.players || []).forEach(p => {
        (p.discards || []).forEach(t => allDiscards.push(t));
      });
      centerPond.innerHTML = allDiscards.map(t =>
        getMahjongTileHTML(t.type || t.Type, t.value ?? t.Value ?? 0, false)
      ).join('');
    }

    // 좌석 배치: 4인 시 top=정면, left=왼쪽, right=오른쪽 / 3인 시 right, top만 사용
    const is3p = Array.isArray(data.players) && data.players.length === 3;
    const players = Array.isArray(data.players) ? [...data.players] : [];
    const myIdx = players.findIndex(p => p && p.userId === currentUserId);
    const seatMap = is3p
      ? { top: (myIdx + 2) % 3, left: -1, right: (myIdx + 1) % 3 }
      : { top: (myIdx + 2) % 4, left: (myIdx + 3) % 4, right: (myIdx + 1) % 4 };

    function renderSeat(playerIdx) {
      const p = playerIdx >= 0 ? players[playerIdx] : null;
      if (!p) return '';
      const meldsHtml = renderMahjongMelds(p.melds);
      const handHtml = Array(p.handCount || 0).fill(0).map(() => getMahjongTileHTML('', 0, true)).join('');
      const name = p.userId ? escapeHTML(p.userId) : '—';
      return `<span class="mahjong-seat-name">${name}</span>
        <div class="mahjong-meld-area">${meldsHtml}</div>
        <div class="mahjong-hand opponent-hand">${handHtml}</div>`;
    }

    const seatTop = document.getElementById('mahjong-seat-top');
    const seatLeft = document.getElementById('mahjong-seat-left');
    const seatRight = document.getElementById('mahjong-seat-right');
    if (seatTop) {
      seatTop.innerHTML = renderSeat(seatMap.top);
      seatTop.classList.toggle('my-turn', players[seatMap.top]?.userId === data.currentTurn);
    }
    if (seatLeft) {
      seatLeft.innerHTML = seatMap.left >= 0 ? renderSeat(seatMap.left) : '';
      seatLeft.classList.toggle('my-turn', seatMap.left >= 0 && players[seatMap.left]?.userId === data.currentTurn);
    }
    if (seatRight) {
      seatRight.innerHTML = renderSeat(seatMap.right);
      seatRight.classList.toggle('my-turn', players[seatMap.right]?.userId === data.currentTurn);
    }

    const seatBottom = document.getElementById('mahjong-seat-bottom');
    if (seatBottom) seatBottom.classList.toggle('my-turn', isMyTurn);

    const meldsMeEl = document.getElementById('mahjong-melds-me');
    const callActionsEl = document.getElementById('mahjong-call-actions');
    const mePlayer = data.players?.find(p => p.userId === currentUserId);
    if (meldsMeEl && mePlayer) {
      meldsMeEl.innerHTML = renderMahjongMelds(mePlayer.melds);
    }

    const isCallWindow = !!data.callWindow;
    const amIDiscarder = data.lastDiscarderId === currentUserId;
    if (callActionsEl) {
      if (isCallWindow && !amIDiscarder) {
        callActionsEl.innerHTML = `
          <button class="mahjong-call-btn chi" onclick="mahjongCall('chi')">치</button>
          <button class="mahjong-call-btn pon" onclick="mahjongCall('pon')">퐁</button>
          <button class="mahjong-call-btn pass" onclick="mahjongCall('pass')">패스</button>`;
        callActionsEl.style.display = 'flex';
      } else {
        callActionsEl.innerHTML = '';
        callActionsEl.style.display = 'none';
      }
    }

    const handEl = document.getElementById('mahjong-hand');
    if (handEl && Array.isArray(myHand)) {
      handEl.innerHTML = myHand.map((t, i) => {
        const discardable = canDiscard ? ' discardable' : '';
        const char = mahjongTileChar(t);
        return `<div class="mahjong-tile${discardable}" data-index="${i}">${char}</div>`;
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

  function renderMahjongMelds(melds) {
    if (!Array.isArray(melds) || melds.length === 0) return '';
    return melds.map(m => {
      const tilesHtml = (m.tiles || []).map(t => getMahjongTileHTML(t.type || t.Type, t.value ?? t.Value ?? 0, false)).join('');
      return `<div class="mahjong-meld-group">${tilesHtml}</div>`;
    }).join('');
  }

  function mahjongDiscard(index) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'discard', index });
  }

  function mahjongCall(callType) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'call', callType });
  }