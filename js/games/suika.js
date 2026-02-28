  // ── 수박게임 (Suika) - 손 커서 + Matter.js + 지분 점수 ─────────────────────────

  const SUIKA_W = 400;
  const SUIKA_H = 500;
  const SUIKA_FORBIDDEN_ZONE = 50;  // 상단 50px 투하 금지선
  const SUIKA_RECHARGE_MS = 3000;
  const SUIKA_MAX_CHARGES = 2;
  const SUIKA_SYNC_INTERVAL_MS = 100;
  const SUIKA_GAMEOVER_THRESHOLD_MS = 3000;
  const SUIKA_FRUIT_NAMES = ['체리','딸기','포도','한라봉','감','사과','배','복숭아','파인애플','멜론','수박'];
  const SUIKA_FRUIT_COLORS = ['#e11d48','#dc2626','#7c3aed','#f97316','#ea580c','#22c55e','#16a34a','#ec4899','#facc15','#84cc16','#15803d'];
  const SUIKA_FRUIT_DEFS = [
    { radius: 12, score: 2 }, { radius: 15, score: 4 }, { radius: 18, score: 8 },
    { radius: 22, score: 16 }, { radius: 26, score: 32 }, { radius: 30, score: 64 },
    { radius: 35, score: 128 }, { radius: 40, score: 256 }, { radius: 46, score: 512 },
    { radius: 52, score: 1024 }, { radius: 60, score: 2048 }
  ];

  let suikaMySlot = -1;
  let suikaChargedCount = 0;
  let suikaLastChargeAt = 0;
  let suikaHandFruitType = -1;
  let suikaGameStarted = false;
  let suikaGameOver = false;
  let suikaIsHost = false;
  let suikaHostUserId = '';
  let suikaChargeInterval = null;
  let suikaSyncInterval = null;
  let suikaEngine = null;
  let suikaRender = null;
  let suikaRunner = null;
  let suikaWorld = null;
  let suikaBodies = {};
  let suikaInitialized = false;
  let suikaPendingMerges = new Set();
  let suikaHandPos = { x: 0, y: 0 };
  let suikaDropX = 200;  // 마우스 X 기준 투하 좌표 (0~400)
  let suikaScores = [0, 0, 0, 0];
  let suikaFruitAboveLineSince = null;  // Host: 과일이 금지선 위에 머문 시각
  let suikaLastDropAt = 0;  // 즉시 드롭용 최소 50ms 간격

  function showSuikaUI() {
    switchGameView('suika');
  }

  function getSuikaChargeProgress() {
    if (suikaChargedCount >= SUIKA_MAX_CHARGES) return 1;
    const now = Date.now();
    const elapsed = now - suikaLastChargeAt;
    return Math.min(1, elapsed / SUIKA_RECHARGE_MS);
  }

  function updateSuikaRechargeBar() {
    const bar = document.getElementById('suika-recharge-bar');
    const fill = document.getElementById('suika-recharge-fill');
    const label = document.getElementById('suika-recharge-label');
    if (!bar || !fill) return;

    const progress = getSuikaChargeProgress();
    fill.style.width = (progress * 100) + '%';

    const canDrop = suikaChargedCount >= 1;
    if (label) {
      label.textContent = canDrop ? `과일 ${suikaChargedCount}/${SUIKA_MAX_CHARGES} 충전됨` : `충전 중... ${Math.ceil((1 - progress) * SUIKA_RECHARGE_MS / 1000)}초`;
    }
    bar.classList.toggle('suika-ready', canDrop);
  }

  function playSuikaChargeSound() {
    if (window.SoundManager) {
      window.SoundManager.playPianoNote(391.99, 0.1);
      setTimeout(() => {
        if (window.SoundManager) window.SoundManager.playPianoNote(523.25, 0.12);
      }, 80);
    }
  }

  function updateSuikaHandCursor() {
    const hand = document.getElementById('suika-hand-cursor');
    const fruitEl = document.getElementById('suika-hand-fruit');
    if (!hand || !fruitEl) return;

    const canShow = suikaGameStarted && !suikaGameOver && suikaChargedCount >= 1 && suikaMySlot >= 0;
    if (!canShow) {
      hand.style.display = 'none';
      return;
    }

    // 손 커서: 상단 50px 투하 금지선 위에서만 움직임 (y는 0~50)
    const y = Math.max(0, Math.min(SUIKA_FORBIDDEN_ZONE - 20, suikaHandPos.y));
    hand.style.display = 'block';
    hand.style.left = suikaHandPos.x + 'px';
    hand.style.top = y + 'px';

    if (suikaHandFruitType >= 0 && suikaHandFruitType < 11) {
      const def = SUIKA_FRUIT_DEFS[suikaHandFruitType];
      fruitEl.style.width = (def.radius * 2) + 'px';
      fruitEl.style.height = (def.radius * 2) + 'px';
      fruitEl.style.borderRadius = '50%';
      fruitEl.style.backgroundColor = SUIKA_FRUIT_COLORS[suikaHandFruitType] || '#888';
      fruitEl.style.display = 'block';
    } else {
      fruitEl.style.display = 'none';
    }

    updateSuikaGhostGuide();
  }

  function initSuikaPhysics(container, data) {
    if (typeof Matter === 'undefined') return;
    const M = Matter;
    const W = SUIKA_W, H = SUIKA_H;

    suikaEngine = M.Engine.create();
    suikaEngine.gravity.y = 0.8;
    suikaEngine.gravity.x = 0;
    suikaWorld = suikaEngine.world;

    const wallThick = 20;
    const wallOpts = { isStatic: true, render: { fillStyle: '#334155' } };
    const walls = [
      M.Bodies.rectangle(-wallThick/2, H/2, wallThick, H+100, wallOpts),
      M.Bodies.rectangle(W+wallThick/2, H/2, wallThick, H+100, wallOpts),
      M.Bodies.rectangle(W/2, H+wallThick/2, W+100, wallThick, wallOpts),
    ];
    M.World.add(suikaWorld, walls);

    suikaRender = M.Render.create({
      element: container,
      engine: suikaEngine,
      options: {
        width: W,
        height: H,
        wireframes: false,
        background: 'linear-gradient(180deg,#1e3a5f 0%,#0f172a 100%)',
      },
    });
    M.Render.run(suikaRender);

    suikaRunner = M.Runner.create();
    M.Runner.run(suikaRunner, suikaEngine);

    // Host: 100ms마다 sync_all 전송
    function tickSync() {
      if (!suikaGameStarted || suikaGameOver || !suikaIsHost || typeof sendGameAction !== 'function') return;
      const bodies = [];
      Object.keys(suikaBodies).forEach(id => {
        const b = suikaBodies[id];
        if (b && b.position) {
          bodies.push({
            id: +id,
            x: b.position.x,
            y: b.position.y,
            vx: b.velocity ? b.velocity.x : 0,
            vy: b.velocity ? b.velocity.y : 0,
          });
        }
      });
      if (bodies.length > 0) sendGameAction({ cmd: 'sync_all', bodies });
    }
    suikaSyncInterval = setInterval(tickSync, SUIKA_SYNC_INTERVAL_MS);

    // Host: 과일이 투하 금지선(50px) 위에 3초 이상 머물면 게임오버
    M.Events.on(suikaEngine, 'afterUpdate', function() {
      if (!suikaIsHost || suikaGameOver) return;
      let anyAbove = false;
      Object.keys(suikaBodies).forEach(id => {
        const b = suikaBodies[id];
        if (b && b.position && b.position.y < SUIKA_FORBIDDEN_ZONE) anyAbove = true;
      });
      const now = Date.now();
      if (anyAbove) {
        if (!suikaFruitAboveLineSince) suikaFruitAboveLineSince = now;
        else if (now - suikaFruitAboveLineSince >= SUIKA_GAMEOVER_THRESHOLD_MS) {
          if (typeof sendGameAction === 'function') sendGameAction({ cmd: 'game_over' });
        }
      } else {
        suikaFruitAboveLineSince = null;
      }
    });

    M.Events.on(suikaEngine, 'collisionStart', function(ev) {
      ev.pairs.forEach(pair => {
        const a = pair.bodyA;
        const b = pair.bodyB;
        if (!a.suikaId || !b.suikaId) return;
        if (a.suikaType !== b.suikaType) return;
        if (a.suikaType >= 10) return;
        const key = [a.suikaId, b.suikaId].sort().join('_');
        if (suikaPendingMerges.has(key)) return;
        suikaPendingMerges.add(key);
        const cx = (a.position.x + b.position.x) / 2;
        const cy = (a.position.y + b.position.y) / 2;
        if (typeof sendGameAction === 'function') {
          sendGameAction({ cmd: 'merge', aid: a.suikaId, bid: b.suikaId, cx, cy });
        }
        if (window.SoundManager) {
          window.SoundManager.playPianoNote(523.25, 0.1);
          setTimeout(() => window.SoundManager?.playPianoNote(783.99, 0.12), 60);
        }
      });
    });

    syncSuikaBodies(data?.fruits || []);
  }

  function syncSuikaBodies(fruits) {
    if (typeof Matter === 'undefined' || !suikaWorld) return;
    const M = Matter;
    const ids = new Set(fruits.map(f => f.id));
    Object.keys(suikaBodies).forEach(id => {
      const numId = +id;
      if (!ids.has(numId)) {
        const b = suikaBodies[id];
        if (b && suikaWorld.bodies.includes(b)) {
          M.Composite.remove(suikaWorld, b);
          const toDel = [];
          suikaPendingMerges.forEach(key => {
            const [a, b] = key.split('_').map(Number);
            if (a === numId || b === numId) toDel.push(key);
          });
          toDel.forEach(k => suikaPendingMerges.delete(k));
        }
        delete suikaBodies[id];
      }
    });
    fruits.forEach(f => {
      if (suikaBodies[f.id]) return;
      const def = SUIKA_FRUIT_DEFS[f.type] || SUIKA_FRUIT_DEFS[0];
      const body = M.Bodies.circle(f.x || 200, f.y || 0, def.radius, {
        friction: 0.1, frictionAir: 0.01, restitution: 0.2,
        render: { fillStyle: SUIKA_FRUIT_COLORS[f.type] || '#888', strokeStyle: 'rgba(255,255,255,0.4)', lineWidth: 2 },
      });
      body.suikaId = f.id;
      body.suikaType = f.type;
      suikaBodies[f.id] = body;
      M.World.add(suikaWorld, body);
    });
  }

  function applySuikaSyncBodies(bodies) {
    if (typeof Matter === 'undefined' || !suikaWorld || suikaIsHost) return;
    const M = Matter;
    bodies.forEach(b => {
      const body = suikaBodies[b.id];
      if (body && M.Bodies.isStatic(body) === false) {
        M.Body.setPosition(body, { x: b.x, y: b.y });
        if (body.velocity) M.Body.setVelocity(body, { x: b.vx || 0, y: b.vy || 0 });
      }
    });
  }

  function addSuikaFruitLocal(fruit) {
    if (typeof Matter === 'undefined' || !suikaWorld) return;
    const M = Matter;
    const def = SUIKA_FRUIT_DEFS[fruit.type] || SUIKA_FRUIT_DEFS[0];
    const body = M.Bodies.circle(fruit.x, 0, def.radius, {
      friction: 0.1, frictionAir: 0.01, restitution: 0.2,
      render: { fillStyle: SUIKA_FRUIT_COLORS[fruit.type] || '#888', strokeStyle: 'rgba(255,255,255,0.4)', lineWidth: 2 },
    });
    body.suikaId = fruit.id;
    body.suikaType = fruit.type;
    suikaBodies[fruit.id] = body;
    M.World.add(suikaWorld, body);
  }

  function renderSuika(data) {
    if (!data) return;

    const players = data.players || [];
    suikaMySlot = players.indexOf(currentUserId);
    suikaGameStarted = data.gameStarted || false;
    suikaGameOver = data.gameOver || false;
    if (suikaGameOver) stopSuikaSyncInterval();
    suikaHostUserId = data.hostUserId || '';
    suikaIsHost = (suikaHostUserId === currentUserId);
    suikaScores = (data.scores || [0, 0, 0, 0]).map(s => typeof s === 'number' ? s : 0);

    const charges = data.charges || [];
    const myCharge = suikaMySlot >= 0 ? charges[suikaMySlot] : null;
    const prevCount = suikaChargedCount;
    if (myCharge) {
      suikaChargedCount = Math.min(SUIKA_MAX_CHARGES, myCharge.chargedCount || 0);
      suikaLastChargeAt = myCharge.lastChargeAt || Date.now();
    }

    if (suikaChargedCount > prevCount && suikaGameStarted) {
      playSuikaChargeSound();
    }
    const nextTypes = data.nextFruitTypes;
    if (suikaMySlot >= 0 && nextTypes && nextTypes[suikaMySlot] !== undefined && suikaChargedCount >= 1) {
      suikaHandFruitType = nextTypes[suikaMySlot];
    } else if (suikaChargedCount < 1) {
      suikaHandFruitType = -1;
    }

    const statusEl = document.getElementById('suika-status');
    if (statusEl) {
      if (!suikaGameStarted) {
        statusEl.textContent = '2명 이상 Ready 시 게임 시작';
      } else if (suikaGameOver) {
        statusEl.textContent = '게임 오버! 과일이 투하 금지선을 3초 이상 넘었습니다.';
      } else {
        statusEl.textContent = suikaChargedCount >= 1 ? '손에 과일이 충전됨! 클릭하여 투하' : '과일 충전 중...';
        if (suikaIsHost) statusEl.textContent += ' [Host]';
      }
    }

    const scoresEl = document.getElementById('suika-scores');
    if (scoresEl && players) {
      const fmt = v => (typeof v === 'number' && v % 1 !== 0) ? v.toFixed(1) : (v || 0);
      scoresEl.innerHTML = players.map((p, i) =>
        p ? `<span class="suika-score" style="color:${['#ef4444','#22c55e','#3b82f6','#eab308'][i]||'#999'}">${escapeHTML(p)}: ${fmt(suikaScores[i])}</span>` : ''
      ).filter(Boolean).join(' | ');
    }

    updateSuikaRechargeBar();
    updateSuikaHandCursor();

    const wrap = document.getElementById('suika-physics-wrap');
    if (!wrap) return;

    if (!suikaInitialized && suikaGameStarted) {
      suikaInitialized = true;
      initSuikaPhysics(wrap, data);
      setupSuikaHandMouse();
      setupSuikaClick();
    } else if (suikaInitialized) {
      syncSuikaBodies(data.fruits || []);
    }
  }

  function setupSuikaHandMouse() {
    const wrap = document.getElementById('suika-game-wrap');
    if (!wrap) return;
    wrap.addEventListener('mousemove', function(e) {
      const rect = wrap.getBoundingClientRect();
      const scaleX = SUIKA_W / rect.width;
      suikaHandPos.x = e.clientX - rect.left - 20;
      suikaHandPos.y = e.clientY - rect.top - 20;
      suikaDropX = Math.max(0, Math.min(SUIKA_W, (e.clientX - rect.left) * scaleX));
      updateSuikaHandCursor();
      updateSuikaGhostGuide();
    });
    wrap.addEventListener('mouseleave', function() {
      const hand = document.getElementById('suika-hand-cursor');
      if (hand) hand.style.display = 'none';
      const ghost = document.getElementById('suika-ghost-guide');
      if (ghost) ghost.style.display = 'none';
    });
  }

  function updateSuikaGhostGuide() {
    const ghostWrap = document.getElementById('suika-ghost-guide');
    const guideLine = document.getElementById('suika-guide-line');
    const ghostFruit = document.getElementById('suika-ghost-fruit');
    if (!ghostWrap || !guideLine || !ghostFruit) return;

    const canShow = suikaGameStarted && !suikaGameOver && suikaChargedCount >= 1 && suikaMySlot >= 0;
    if (!canShow) {
      ghostWrap.style.display = 'none';
      return;
    }

    ghostWrap.style.display = 'block';
    guideLine.style.left = suikaDropX + 'px';
    ghostFruit.style.left = suikaDropX + 'px';

    const type = (suikaHandFruitType >= 0 && suikaHandFruitType < 11) ? suikaHandFruitType : 0;
    const def = SUIKA_FRUIT_DEFS[type];
    const r = def.radius;
    ghostFruit.style.width = (r * 2) + 'px';
    ghostFruit.style.height = (r * 2) + 'px';
    ghostFruit.style.backgroundColor = SUIKA_FRUIT_COLORS[type] || 'rgba(255,255,255,0.25)';
  }

  function setupSuikaClick() {
    const wrap = document.getElementById('suika-game-wrap');
    if (!wrap) return;
    function doDrop() {
      if (suikaMySlot < 0 || suikaChargedCount < 1 || suikaGameOver) return;
      const now = Date.now();
      if (now - suikaLastDropAt < 50) return;  // 50ms 최소 간격
      suikaLastDropAt = now;
      const type = (suikaHandFruitType >= 0 && suikaHandFruitType <= 3) ? suikaHandFruitType : -1;
      const payload = type >= 0 ? { cmd: 'drop', x: suikaDropX, type } : { cmd: 'drop', x: suikaDropX };
      if (typeof sendGameAction === 'function') {
        sendGameAction(payload, { skipCooldown: true, cooldownMs: 50 });
      }
    }
    wrap.addEventListener('pointerdown', function(e) {
      e.preventDefault();
      doDrop();
    });
  }

  function handleSuikaDropResult(success) {
    if (success) {
      if (window.SoundManager) window.SoundManager.playPianoNote(261.63, 0.08);
    }
  }

  function startSuikaChargeTick() {
    stopSuikaChargeTick();
    suikaChargeInterval = setInterval(() => {
      if (!document.getElementById('suika-container')?.offsetParent) return;
      updateSuikaRechargeBar();
      updateSuikaHandCursor();
    }, 100);
  }

  function stopSuikaChargeTick() {
    if (suikaChargeInterval) {
      clearInterval(suikaChargeInterval);
      suikaChargeInterval = null;
    }
  }

  function handleSuikaSyncAll(bodies) {
    applySuikaSyncBodies(bodies);
  }

  function stopSuikaSyncInterval() {
    if (suikaSyncInterval) {
      clearInterval(suikaSyncInterval);
      suikaSyncInterval = null;
    }
  }

  window.showSuikaUI = showSuikaUI;
  window.renderSuika = renderSuika;
  window.suikaOnDropResult = handleSuikaDropResult;
  window.suikaOnSyncAll = handleSuikaSyncAll;

  (function() {
    const obs = new MutationObserver(() => {
      const container = document.getElementById('suika-container');
      if (container?.offsetParent) {
        startSuikaChargeTick();
      } else {
        stopSuikaChargeTick();
      }
    });
    obs.observe(document.body, { childList: true, subtree: true });
  })();
