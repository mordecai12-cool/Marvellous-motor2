(() => {
  if (window.matchMedia('(max-width: 720px)').matches) {
    const breakdown = document.querySelector('.payment-breakdown');
    if (breakdown) breakdown.open = false;
  }
  const feedback = document.querySelector('[data-copy-feedback]');
  document.querySelectorAll('[data-copy-value]').forEach(button => {
    button.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(button.dataset.copyValue);
        feedback.textContent = button.dataset.copyValue + ' copied to clipboard.';
      } catch {
        feedback.textContent = 'Copy is unavailable in this browser. Select and copy the details above.';
      }
    });
  });
  const receipt = document.querySelector('#receipt');
  const selection = document.querySelector('#receipt-selection');
  if (!receipt || !selection) return;
  receipt.addEventListener('change', () => {
    const file = receipt.files[0];
    receipt.setCustomValidity('');
    selection.classList.remove('invalid');
    if (!file) { selection.textContent = 'No file selected yet.'; return; }
    let error = '';
    if (file.size > 5 * 1024 * 1024) error = 'Choose a receipt smaller than or equal to 5 MB.';
    else if (!['image/png', 'image/jpeg', 'application/pdf'].includes(file.type)) error = 'Choose a PNG, JPEG or PDF receipt.';
    receipt.setCustomValidity(error);
    selection.classList.toggle('invalid', Boolean(error));
    selection.textContent = error || file.name + ' · ' + (file.size < 1024 * 1024 ? Math.max(1, Math.ceil(file.size / 1024)) + ' KB' : (file.size / (1024 * 1024)).toFixed(1) + ' MB') + ' · Ready to submit';
  });
})();
