// General settings use the current management session; the note key stays separate.
(() => {
  const button = document.getElementById('openGeneralSettings');
  button.disabled = true;
  window.toolboxFeedback.connect(location.href).then(() => {
    button.disabled = false;
  }).catch(() => {
    button.title = '设置连接失败，请刷新页面后重试';
  });
})();
