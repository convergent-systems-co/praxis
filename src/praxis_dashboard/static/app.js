(function () {
  'use strict';

  var POLL_INTERVAL_MS = 2000;
  var replayMode = false;

  function escapeHtml(value) {
    return String(value)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }

  function fmt(value) {
    if (value === null || value === undefined) return '';
    return escapeHtml(value);
  }

  function renderRunSummary(runSummary) {
    var el = document.getElementById('run-summary-content');
    if (!runSummary) {
      el.innerHTML = '';
      return;
    }
    var counts = Object.keys(runSummary.counts_by_status || {})
      .map(function (status) {
        return fmt(status) + ': ' + fmt(runSummary.counts_by_status[status]);
      })
      .join(', ');
    el.innerHTML =
      '<div>Run: ' + fmt(runSummary.run_id) + '</div>' +
      '<div>Total nodes: ' + fmt(runSummary.total_nodes) + '</div>' +
      '<div>Status counts: ' + counts + '</div>' +
      '<div>Complete: ' + fmt(runSummary.is_complete) + '</div>';
  }

  function renderNodes(nodes) {
    var el = document.getElementById('nodes-list');
    el.innerHTML = (nodes || [])
      .map(function (node) {
        var blocked = node.blocked_reason
          ? ' &mdash; blocked: ' + fmt(node.blocked_reason)
          : '';
        return (
          '<li>' +
          fmt(node.node_id) + ' [' + fmt(node.kind) + '] status: ' + fmt(node.status) +
          ' next: ' + fmt((node.legal_next_events || []).join(', ')) +
          blocked +
          '</li>'
        );
      })
      .join('');
  }

  function renderNextActions(nextActions) {
    var el = document.getElementById('next-actions-list');
    el.innerHTML = (nextActions || [])
      .map(function (action) {
        return '<li>' + fmt(action) + '</li>';
      })
      .join('');
  }

  function renderEvidence(evidence) {
    var el = document.getElementById('evidence-list');
    el.innerHTML = (evidence || [])
      .map(function (item) {
        var flag = item.satisfied === false ? ' <strong>NOT SATISFIED</strong>' : '';
        var reasons = (item.reasons || []).length
          ? ' reasons: ' + fmt(item.reasons.join('; '))
          : '';
        var stale = item.stale_warning ? ' <em>' + fmt(item.stale_warning) + '</em>' : '';
        return (
          '<li>' +
          fmt(item.node_id) + ' requires ' + fmt((item.required_proof_types || []).join(', ')) +
          flag + reasons + stale +
          '</li>'
        );
      })
      .join('');
  }

  function renderResources(resources) {
    var el = document.getElementById('resources-list');
    el.innerHTML = (resources || [])
      .map(function (lease) {
        var stale = lease.stale_warning ? ' <em>' + fmt(lease.stale_warning) + '</em>' : '';
        var expired = lease.expired ? ' (expired)' : '';
        return (
          '<li>' +
          fmt(lease.resource_type) + ' ' + fmt(lease.identifier) +
          ' owner: ' + fmt(lease.owner) +
          ' mode: ' + fmt(lease.access_mode) +
          ' epoch: ' + fmt(lease.epoch) +
          expired + stale +
          '</li>'
        );
      })
      .join('');
  }

  function renderExecutorAssignments(assignments) {
    var el = document.getElementById('executor-assignments-list');
    el.innerHTML = (assignments || [])
      .map(function (assignment) {
        return (
          '<li>' +
          fmt(assignment.node_id) + ' / ' + fmt(assignment.proof_type) +
          ' executor: ' + fmt(assignment.executor_id) +
          ' grader: ' + fmt(assignment.grader_kind) +
          ' status: ' + fmt(assignment.status) +
          '</li>'
        );
      })
      .join('');
  }

  function renderCapabilities(capabilities) {
    var el = document.getElementById('capabilities-list');
    el.innerHTML = (capabilities || [])
      .map(function (capability) {
        return (
          '<li>' +
          fmt(capability.executor_id) +
          ' handles: ' + fmt((capability.satisfied_kinds || []).join(', ')) +
          ' cost hint: ' + fmt(capability.cost_hint) +
          '</li>'
        );
      })
      .join('');
  }

  function renderMetrics(metricsList) {
    var el = document.getElementById('metrics-list');
    el.innerHTML = (metricsList || [])
      .map(function (item) {
        var confidence = Object.keys(item.evidence_confidence || {})
          .map(function (kind) {
            return fmt(kind) + ': ' + fmt(item.evidence_confidence[kind]);
          })
          .join(', ');
        return (
          '<li>' +
          fmt(item.node_id) +
          ' retries: ' + fmt(item.retry_count) +
          ' handoffs: ' + fmt(item.handoff_count) +
          ' confidence: ' + confidence +
          '</li>'
        );
      })
      .join('');
  }

  function renderWarnings(warnings) {
    var el = document.getElementById('warnings-banner');
    var list = warnings || [];
    if (list.length === 0) {
      el.innerHTML = '';
      el.hidden = true;
      return;
    }
    el.hidden = false;
    el.innerHTML = list.map(fmt).join(' | ');
  }

  function renderSnapshot(document_) {
    renderRunSummary(document_.run_summary);
    renderNodes(document_.nodes);
    renderNextActions(document_.next_actions);
    renderEvidence(document_.evidence);
    renderResources(document_.resources);
    renderExecutorAssignments(document_.executor_assignments);
    renderCapabilities(document_.capabilities);
    renderMetrics(document_.metrics);
    renderWarnings(document_.warnings);
  }

  function poll() {
    var url = '/api/snapshot' + (replayMode ? '?replay=1' : '');
    fetch(url)
      .then(function (response) {
        return response.json();
      })
      .then(renderSnapshot)
      .catch(function (error) {
        console.error('snapshot poll failed', error);
      });
  }

  function toggleReplay(event) {
    if (event && typeof event.preventDefault === 'function') event.preventDefault();
    replayMode = !replayMode;
    var button = document.getElementById('replay-toggle');
    button.textContent = replayMode ? 'Replay: On' : 'Replay: Off';
  }

  function init() {
    var button = document.getElementById('replay-toggle');
    button.addEventListener('click', toggleReplay);
    poll();
    setInterval(poll, POLL_INTERVAL_MS);
  }

  document.addEventListener('DOMContentLoaded', init);
})();
