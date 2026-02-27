  // ── 수박게임 (Suika) - 손 커서 + Matter.js + 지분 점수 ─────────────────────────

  const SUIKA_W = 400;
  const SUIKA_H = 500;
  const SUIKA_RECHARGE_MS = 3000;
  const SUIKA_MAX_CHARGES = 2;
  const SUIKA_FRUIT_NAMES = ['체리','딸기','포도','데코폰','오렌지','사과','배','복숭아','파인애플','멜론','수박'];
  const SUIKA_FRUIT_COLORS = ['#e11d48','#dc2626','#7c3aed','#f97316','#ea580c','#22c55e','#16a34a','#ec4899','#facc15','#84cc16','#15803d'];
  const SUIKA_FRUIT_DEFS = [
    { radius: 12, score: 1 }, { radius: 15, score: 3 }, { radius: 18, score: 6 },
    { radius: 22, score: 10 }, { radius: 26, score: 15 }, { radius: 30, score: 21 },
    { radius: 35, score: 28 }, { radius: 40, score: 36 }, { radius: 46, score: 45 },
    { radius: 52, score: 55 }, { radius: 60, score: 66 }
  ];

  let suikaMySlot = -1;
  let suikaChargedCount = 0;
  let suikaLastChargeAt = 0;
  let suikaHandFruitType = -1;
  let suikaGameStarted = false;
  let suikaChargeInterval = null;
  let suikaEngine = null;
  let suikaRender = null;
  let suikaRunner = null;
  let suikaWorld = null;
  let suikaBodies = {};
  let suikaInitialized = false;
  let suikaPendingMerges = new Set();
  let suikaHandPos = { x: 0, y: 0 };
  let suikaScores = [0, 0, 0, 0];

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

    const canShow = suikaGameStarted && suikaChargedCount >= 1 && suikaMySlot >= 0;
    if (!canShow) {
      hand.style.display = 'none';
      return;
    }

    hand.style.display = 'block';
    hand.style.left = suikaHandPos.x + 'px';
    hand.style.top = suikaHandPos.y + 'px';

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
    suikaScores = data.scores || [0, 0, 0, 0];

    const charges = data.charges || [];
    const myCharge = suikaMySlot >= 0 ? charges[suikaMySlot] : null;
    const prevCount = suikaChargedCount;
    if (myCharge) {
      suikaChargedCount = Math.min(SUIKA_MAX_CHARGES, myCharge.chargedCount || 0);
      suikaLastChargeAt = myCharge.lastChargeAt || Date.now();
    }

    if (suikaChargedCount > prevCount && suikaGameStarted) {
      suikaHandFruitType = Math.floor(Math.random() * 4);
      playSuikaChargeSound();
    }
    if (suikaChargedCount < 1) suikaHandFruitType = -1;

    const statusEl = document.getElementById('suika-status');
    if (statusEl) {
      if (!suikaGameStarted) {
        statusEl.textContent = '2명 이상 Ready 시 게임 시작';
      } else {
        statusEl.textContent = suikaChargedCount >= 1 ? '손에 과일이 충전됨! 클릭하여 투하' : '과일 충전 중...';
      }
    }

    const scoresEl = document.getElementById('suika-scores');
    if (scoresEl && players) {
      scoresEl.innerHTML = players.map((p, i) =>
        p ? `<span class="suika-score" style="color:${['#ef4444','#22c55e','#3b82f6','#eab308'][i]||'#999'}">${escapeHTML(p)}: ${suikaScores[i]||0}</span>` : ''
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
      suikaHandPos.x = e.clientX - rect.left - 20;
      suikaHandPos.y = e.clientY - rect.top - 20;
      updateSuikaHandCursor();
    });
    wrap.addEventListener('mouseleave', function() {
      const hand = document.getElementById('suika-hand-cursor');
      if (hand) hand.style.display = 'none';
    });
  }

  function setupSuikaClick() {
    const wrap = document.getElementById('suika-physics-wrap');
    if (!wrap) return;
    wrap.addEventListener('click', function(e) {
      if (suikaMySlot < 0 || suikaChargedCount < 1) return;
      const rect = wrap.getBoundingClientRect();
      const scaleX = SUIKA_W / rect.width;
      const x = (e.clientX - rect.left) * scaleX;
      if (typeof sendGameAction === 'function') {
        sendGameAction({ cmd: 'drop', x });
      }
    });
  }

  function handleSuikaDropResult(success) {
    if (success) {
      suikaHandFruitType = -1;
      updateSuikaHandCursor();
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

  window.showSuikaUI = showSuikaUI;
  window.renderSuika = renderSuika;
  window.suikaOnDropResult = handleSuikaDropResult;

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
