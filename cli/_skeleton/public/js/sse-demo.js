(function () {
  const statusEl = document.getElementById('sse-status');
  const logEl = document.getElementById('sse-log');
  const pingBtn = document.getElementById('sse-ping');

  function setStatus(status) {
    if (!statusEl) return;
    statusEl.textContent = status;
    statusEl.setAttribute('data-status', status);
  }

  function appendLog(text) {
    if (!logEl) return;
    const item = document.createElement('li');
    item.textContent = text;
    logEl.insertBefore(item, logEl.firstChild);
    while (logEl.children.length > 5) {
      logEl.removeChild(logEl.lastChild);
    }
  }

  function connect() {
    setStatus('connecting');
    const source = new EventSource('/sse/stream');

    source.onopen = function () {
      setStatus('open');
    };

    source.onerror = function () {
      setStatus('error');
    };

    source.addEventListener('heartbeat', function (e) {
      try {
        const data = JSON.parse(e.data);
        appendLog('heartbeat: ' + data.time);
      } catch (err) {
        appendLog('heartbeat: ' + e.data);
      }
    });

    source.addEventListener('ping', function (e) {
      try {
        const data = JSON.parse(e.data);
        appendLog('ping: ' + data.message + ' at ' + data.time);
      } catch (err) {
        appendLog('ping: ' + e.data);
      }
    });
  }

  if (pingBtn) {
    pingBtn.addEventListener('click', function () {
      fetch('/sse/ping')
        .then(function (response) { return response.json(); })
        .then(function (data) { console.log('ping response', data); })
        .catch(function (err) { console.error('ping error', err); });
    });
  }

  connect();
})();
