// Minimal self-contained confetti burst (no external dependencies, CSP-safe).
// Exposes window.burstConfetti().
(function () {
  function burst() {
    var layer = document.getElementById('confetti-layer');
    if (!layer) return;
    var canvas = document.createElement('canvas');
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
    canvas.style.width = '100%';
    canvas.style.height = '100%';
    layer.appendChild(canvas);
    var ctx = canvas.getContext('2d');

    var colors = ['#2ecc71', '#f1c40f', '#5865f2', '#e74c3c', '#ffffff'];
    var pieces = [];
    for (var i = 0; i < 160; i++) {
      pieces.push({
        x: canvas.width / 2,
        y: canvas.height / 3,
        vx: (Math.random() - 0.5) * 16,
        vy: Math.random() * -14 - 4,
        size: Math.random() * 8 + 4,
        color: colors[Math.floor(Math.random() * colors.length)],
        rot: Math.random() * Math.PI,
        vr: (Math.random() - 0.5) * 0.3
      });
    }

    var start = null;
    function frame(ts) {
      if (!start) start = ts;
      var elapsed = ts - start;
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      for (var i = 0; i < pieces.length; i++) {
        var p = pieces[i];
        p.vy += 0.35; // gravity
        p.x += p.vx;
        p.y += p.vy;
        p.rot += p.vr;
        ctx.save();
        ctx.translate(p.x, p.y);
        ctx.rotate(p.rot);
        ctx.fillStyle = p.color;
        ctx.fillRect(-p.size / 2, -p.size / 2, p.size, p.size * 0.6);
        ctx.restore();
      }
      if (elapsed < 3500) {
        requestAnimationFrame(frame);
      } else {
        layer.removeChild(canvas);
      }
    }
    requestAnimationFrame(frame);
  }

  window.burstConfetti = burst;
})();
