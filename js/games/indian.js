  // ── Indian Poker UI ────────────────────────────────────────────────────────

  // Indian Poker 카드 렌더링 최적화 (변경 시에만 업데이트)
  let lastIndianOppCard = '';
  let lastIndianMyCard  = '';

  function showIndianUI() {
    lastIndianOppCard = '';
    lastIndianMyCard  = '';
    switchGameView('indian');
  }

  /** 하트 바 HTML 생성 (기본 10칸, 10개 초과 시 최대 20개까지 아이콘 확장) */
  function renderHeartsBar(count) {
    const MAX_ICONS = 20; // 아이콘으로 표시할 최대 개수
    const displayMax = Math.max(10, count); // 기본 10칸 유지, 넘으면 그만큼 슬롯 확장

    let html = '';
    const iconCount = Math.min(count, MAX_ICONS);
    const totalSlots = Math.min(displayMax, MAX_ICONS);

    for (let i = 0; i < totalSlots; i++) {
      html += `<span class="indian-heart${i >= iconCount ? ' lost' : ''}">❤️</span>`;
    }
    if (count > MAX_ICONS) {
      html += `<span class="indian-hearts-count">+${count - MAX_ICONS}</span>`;
    }
    return html;
  }

  /** 카드 HTML 생성 */
  function renderIndianCard(card) {
    if (!card || card.hidden) {
      // 뒷면
      return `<div class="indian-card-back">🃏</div>`;
    }
    const isRed = card.suit === '♥' || card.suit === '♦';
    return `
      <div class="indian-card-face ${isRed ? 'red-suit' : 'black-suit'}">
        <div class="indian-card-val-top">${card.value}</div>
        <div class="indian-card-suit-center">${card.suit}</div>
        <div class="indian-card-val-bot">${card.value}</div>
      </div>`;
  }

  function renderIndian(data) {
    if (!data) return;

    const isMyTurn = data.turn === currentUserId;
    const isSpectator = data.myName !== currentUserId && data.opponentName !== currentUserId;

    // 라운드 바
    const roundEl = document.getElementById('indian-round-bar');
    if (roundEl) roundEl.textContent = `라운드 ${data.round}`;

    // 상태 바
    const statusEl = document.getElementById('indian-status');
    if (statusEl) {
      if (data.phase === 'waiting') {
        statusEl.textContent = '상대방을 기다리는 중...';
      } else if (isMyTurn && !isSpectator) {
        statusEl.textContent = data.phase === 'first_action'
          ? '🎯 내 차례 — 승부 또는 포기를 선택하세요!'
          : '🎯 내 차례 — 콜(승부) 또는 포기를 선택하세요!';
        statusEl.style.color = 'var(--accent)';
      } else {
        statusEl.innerHTML = `⏳ <span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(data.turn)}')" title="전적 보기">${escapeHTML(data.turn)}</span>의 선택을 기다리는 중...`;
        statusEl.style.color = 'var(--text-secondary)';
      }
    }

    // 상대방 정보
    const oppName = data.opponentName || '—';
    document.getElementById('indian-opp-name').innerHTML = oppName !== '—'
      ? `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(oppName)}')" title="전적 보기">${escapeHTML(oppName)}</span>`
      : '—';
    document.getElementById('indian-opp-hearts').innerHTML = renderHeartsBar(data.opponentHearts);
    const oppCardHtml = renderIndianCard(data.opponentCard);
    if (lastIndianOppCard !== oppCardHtml) {
      const oppWrap = document.getElementById('indian-opp-card-wrap');
      oppWrap.innerHTML = oppCardHtml;
      lastIndianOppCard = oppCardHtml;
      const cardEl = oppWrap.querySelector('.indian-card-face, .indian-card-back');
      if (cardEl && window.applyCardFlipAnim) window.applyCardFlipAnim(cardEl);
    }

    // 내 정보
    const myName = data.myName || '—';
    document.getElementById('indian-my-name').innerHTML = myName !== '—'
      ? `<span class="clickable-nickname" onclick="requestOpponentRecord('${escapeForJsAttr(myName)}')" title="전적 보기">${escapeHTML(myName)}</span>`
      : '—';
    document.getElementById('indian-my-hearts').innerHTML = renderHeartsBar(data.myHearts);
    if ((data.phase === 'game_over' || data.phase === 'settlement' || data.phase === 'showdown') && data.myCard) {
      data.myCard.hidden = false;
    }
    const myCardHtml = renderIndianCard(data.myCard);
    if (lastIndianMyCard !== myCardHtml) {
      const myWrap = document.getElementById('indian-my-card-wrap');
      myWrap.innerHTML = myCardHtml;
      lastIndianMyCard = myCardHtml;
      const cardEl = myWrap.querySelector('.indian-card-face, .indian-card-back');
      if (cardEl && window.applyCardFlipAnim) window.applyCardFlipAnim(cardEl);
    }

    // 현재 턴 유저 래퍼에 active-turn 강조
    const oppArea = document.querySelector('.indian-player-area.opponent-area');
    const myArea = document.querySelector('.indian-player-area.my-area');
    if (oppArea) oppArea.classList.toggle('active-turn', data.turn === data.opponentName);
    if (myArea) myArea.classList.toggle('active-turn', data.turn === data.myName);

    // 액션 버튼 활성화 (내 턴이고 관전자가 아닐 때)
    const canAct = isMyTurn && !isSpectator && data.phase !== 'waiting';
    document.getElementById('btn-indian-showdown').disabled = !canAct;
    document.getElementById('btn-indian-giveup').disabled   = !canAct;
  }

  function indianShowdown() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'showdown' });
  }

  function indianGiveUp() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    sendGameAction({ cmd: 'give_up' });
  }

  /** 인디언 포커 승부 결과 오버레이 — 4초간 표시 후 자동 닫힘 */
  let indianShowdownTimeout = null;
  function showIndianShowdownOverlay(data) {
    if (!data) return;
    const box   = document.getElementById('indian-showdown-box');
    const vsEl  = document.getElementById('indian-showdown-vs');
    const resEl = document.getElementById('indian-showdown-result');
    const overlay = document.getElementById('indian-showdown-overlay');
    if (!box || !vsEl || !resEl || !overlay) return;

    // 오버레이 표시 시 게임 판 위의 내 카드·상대 카드도 공개
    const myWrap = document.getElementById('indian-my-card-wrap');
    const oppWrap = document.getElementById('indian-opp-card-wrap');
    if (myWrap && data.myCard) {
      const myCardHtml = renderIndianCard({ ...data.myCard, hidden: false });
      myWrap.innerHTML = myCardHtml;
      lastIndianMyCard = myCardHtml;
      const myCardEl = myWrap.querySelector('.indian-card-face, .indian-card-back');
      if (myCardEl && window.applyCardFlipAnim) window.applyCardFlipAnim(myCardEl);
    }
    if (oppWrap && data.opponentCard) {
      const oppCardHtml = renderIndianCard({ ...data.opponentCard, hidden: false });
      oppWrap.innerHTML = oppCardHtml;
      lastIndianOppCard = oppCardHtml;
      const oppCardEl = oppWrap.querySelector('.indian-card-face, .indian-card-back');
      if (oppCardEl && window.applyCardFlipAnim) window.applyCardFlipAnim(oppCardEl);
    }

    const myVal = data.myCard?.value ?? '?';
    const oppVal = data.opponentCard?.value ?? '?';
    const mySuit = data.myCard?.suit ?? '';
    const oppSuit = data.opponentCard?.suit ?? '';
    const result = data.result || '';
    const delta = data.heartDelta ?? 0;
    const deltaStr = delta >= 0 ? `+${delta}` : `${delta}`;

    vsEl.textContent = `내 카드 ${myVal}${mySuit}  vs  상대 ${oppVal}${oppSuit}`;
    let resText, isWin;
    if (result === 'giveup') {
      resText = '포기 (-1 하트)';
      isWin = false;
    } else if (result === 'win' && delta === 0) {
      resText = '상대방 포기 (+0 하트)';
      isWin = true;
    } else {
      resText = result === 'win' ? `승리! (${deltaStr} 하트)` : `패배 (${deltaStr} 하트)`;
      isWin = result === 'win';
    }
    resEl.textContent = resText;
    resEl.className = 'indian-showdown-result ' + (isWin ? 'win' : 'lose');
    box.className = 'unified-result-box ' + (isWin ? 'win' : 'lose');

    if (indianShowdownTimeout) clearTimeout(indianShowdownTimeout);
    overlay.classList.add('show');
    indianShowdownTimeout = setTimeout(() => {
      overlay.classList.remove('show');
      indianShowdownTimeout = null;
    }, 4000);
  }

  function closeIndianOverlay() {
    const overlay = document.getElementById('indian-showdown-overlay');
    if (overlay) overlay.classList.remove('show');
    if (indianShowdownTimeout) {
      clearTimeout(indianShowdownTimeout);
      indianShowdownTimeout = null;
    }
  }