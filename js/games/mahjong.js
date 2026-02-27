  // ── Mahjong (마작) UI ───────────────────────────────────────────────────────

  function showMahjongUI() {
    switchGameView('mahjong');
  }

  /** 서버 패 데이터(type, value)를 CSS 클래스명으로 매핑 */
  function mapValueToClassName(tile) {
    if (!tile || !tile.type) return '';
    const type = tile.type;
    const value = tile.value ?? tile.Value ?? 0;
    if (type === 'man' && value >= 1 && value <= 9) return 'm-' + value + 'm';
    if (type === 'pin' && value >= 1 && value <= 9) return 'm-' + value + 'p';
    if (type === 'sou' && value >= 1 && value <= 9) return 'm-' + value + 's';
    if (type === 'honor' && value >= 1 && value <= 7) {
      const honorNames = ['', 'east', 'south', 'west', 'north', 'white', 'green', 'red'];
      return 'm-' + honorNames[value];
    }
    return '';
  }

  /** DOM 기반 마작 패 엘리먼트 생성 (innerHTML 대신 appendChild용) */
  function createTileDOM(tile, isHidden, isDiscardable, index) {
    const el = document.createElement('div');
    el.className = 'mj-tile';
    if (isHidden) {
      el.classList.add('hidden');
    } else {
      const cls = mapValueToClassName(tile);
      if (cls) el.classList.add(cls);
    }
    if (isDiscardable) {
      el.classList.add('discardable');
      el.style.cursor = 'pointer';
      el.dataset.value = JSON.stringify(tile);
      if (typeof index === 'number') el.dataset.index = String(index);
    }
    return el;
  }

  function clearAndAppend(parent, children) {
    while (parent.firstChild) parent.removeChild(parent.firstChild);
    if (Array.isArray(children)) {
      children.forEach(c => parent.appendChild(c));
    }
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

    const centerPond = document.getElementById('mahjong-center-pond');
    if (centerPond) clearAndAppend(centerPond, []);

    const is3p = Array.isArray(data.players) && data.players.length === 3;
    const players = Array.isArray(data.players) ? [...data.players] : [];
    const myIdx = players.findIndex(p => p && p.userId === currentUserId);
    const seatMap = is3p
      ? { top: (myIdx + 2) % 3, left: -1, right: (myIdx + 1) % 3 }
      : { top: (myIdx + 2) % 4, left: (myIdx + 3) % 4, right: (myIdx + 1) % 4 };

    function appendSeatContent(container, playerIdx) {
      const p = playerIdx >= 0 ? players[playerIdx] : null;
      if (!p) return;
      const isSideSeat = container.classList.contains('seat-left') || container.classList.contains('seat-right');

      const nameSpan = document.createElement('span');
      nameSpan.className = 'mahjong-seat-name';
      nameSpan.textContent = p.userId ? p.userId : '—';

      const discardsDiv = document.createElement('div');
      discardsDiv.className = 'mahjong-discards' + (isSideSeat ? ' side-seat-discards' : '');
      (p.discards || []).forEach(t => {
        discardsDiv.appendChild(createTileDOM(t, false, false));
      });

      const meldArea = document.createElement('div');
      meldArea.className = 'mahjong-meld-area';
      (p.melds || []).forEach(m => {
        const group = document.createElement('div');
        group.className = 'mahjong-meld-group';
        (m.tiles || []).forEach(t => {
          group.appendChild(createTileDOM(t, false, false));
        });
        meldArea.appendChild(group);
      });

      const handDiv = document.createElement('div');
      handDiv.className = 'mahjong-hand opponent-hand' + (isSideSeat ? ' side-seat-hand' : '');
      for (let i = 0; i < (p.handCount || 0); i++) {
        handDiv.appendChild(createTileDOM({ type: '', value: 0 }, true, false));
      }

      clearAndAppend(container, [nameSpan, discardsDiv, meldArea, handDiv]);
    }

    const seatTop = document.getElementById('mahjong-seat-top');
    const seatLeft = document.getElementById('mahjong-seat-left');
    const seatRight = document.getElementById('mahjong-seat-right');
    if (seatTop) {
      clearAndAppend(seatTop, []);
      if (seatMap.top >= 0) appendSeatContent(seatTop, seatMap.top);
      seatTop.classList.toggle('my-turn', players[seatMap.top]?.userId === data.currentTurn);
    }
    if (seatLeft) {
      clearAndAppend(seatLeft, []);
      if (seatMap.left >= 0) appendSeatContent(seatLeft, seatMap.left);
      seatLeft.classList.toggle('my-turn', seatMap.left >= 0 && players[seatMap.left]?.userId === data.currentTurn);
    }
    if (seatRight) {
      clearAndAppend(seatRight, []);
      if (seatMap.right >= 0) appendSeatContent(seatRight, seatMap.right);
      seatRight.classList.toggle('my-turn', players[seatMap.right]?.userId === data.currentTurn);
    }

    const seatBottom = document.getElementById('mahjong-seat-bottom');
    if (seatBottom) seatBottom.classList.toggle('my-turn', isMyTurn);

    const mePlayer = data.players?.find(p => p.userId === currentUserId);
    const discardsMeEl = document.getElementById('mahjong-discards-me');
    const meldsMeEl = document.getElementById('mahjong-melds-me');
    const callActionsEl = document.getElementById('mahjong-call-actions');
    if (discardsMeEl && mePlayer) {
      clearAndAppend(discardsMeEl, (mePlayer.discards || []).map(t =>
        createTileDOM(t, false, false)
      ));
    }
    if (meldsMeEl && mePlayer) {
      const meldChildren = [];
      (mePlayer.melds || []).forEach(m => {
        const group = document.createElement('div');
        group.className = 'mahjong-meld-group';
        (m.tiles || []).forEach(t => {
          group.appendChild(createTileDOM(t, false, false));
        });
        meldChildren.push(group);
      });
      clearAndAppend(meldsMeEl, meldChildren);
    }

    const isCallWindow = !!data.callWindow;
    const amIDiscarder = data.lastDiscarderId === currentUserId;
    if (callActionsEl) {
      clearAndAppend(callActionsEl, []);
      if (isCallWindow && !amIDiscarder) {
        ['chi', 'pon', 'pass'].forEach(ct => {
          const btn = document.createElement('button');
          btn.className = 'mahjong-call-btn ' + ct;
          btn.textContent = ct === 'chi' ? '치' : ct === 'pon' ? '퐁' : '패스';
          btn.onclick = () => mahjongCall(ct);
          callActionsEl.appendChild(btn);
        });
        callActionsEl.style.display = 'flex';
      } else {
        callActionsEl.style.display = 'none';
      }
    }

    const handEl = document.getElementById('mahjong-hand');
    if (handEl && Array.isArray(myHand)) {
      clearAndAppend(handEl, myHand.map((t, i) => {
        const tileEl = createTileDOM(t, false, canDiscard, i);
        if (canDiscard) {
          tileEl.onclick = () => {
            const idx = parseInt(tileEl.dataset.index, 10);
            if (!isNaN(idx)) mahjongDiscard(idx);
          };
        }
        return tileEl;
      }));
    }
  }

  function mahjongDiscard(index) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'discard', index });
  }

  function mahjongCall(callType) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'call', callType });
  }
