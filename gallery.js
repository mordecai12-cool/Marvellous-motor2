(() => {
  const gallery = document.querySelector('[data-gallery]');
  if (!gallery) return;
  const buttons = [...gallery.querySelectorAll('[data-photo]')];
  const image = gallery.querySelector('.gallery-main-image');
  const caption = gallery.querySelector('[data-photo-caption]');
  const credit = gallery.querySelector('[data-photo-credit]');
  const dialog = gallery.querySelector('dialog');
  const enlarged = dialog.querySelector('img');
  const label = dialog.querySelector('[data-enlarged-label]');
  let index = 0;
  function select(next) {
    index = (next + buttons.length) % buttons.length;
    const button = buttons[index];
    image.src = button.dataset.photo;
    const actual = gallery.dataset.actual === "true";
    image.alt = `${gallery.dataset.vehicle} · ${button.dataset.label} · ${actual ? "actual stock vehicle" : "representative model photograph"}`;
    caption.textContent = actual
      ? `${button.dataset.label} · Stock photograph supplied by the dealership`
      : button.dataset.reference === "true"
        ? `${button.dataset.label} · Separate model/trim reference; not this exact vehicle`
        : `${button.dataset.label} · Representative model photograph`;
    if(button.dataset.retouched === 'true') caption.textContent += ' · Retouched to remove people';
    credit.href = button.dataset.source;
    credit.textContent = button.dataset.credit || 'Photo credits';
    buttons.forEach((item, i) => item.setAttribute('aria-pressed', String(i === index)));
    enlarged.src = image.src;
    enlarged.alt = image.alt;
    label.textContent = `${index + 1} / ${buttons.length} · ${button.dataset.label}`;
  }
  buttons.forEach((button, i) => button.addEventListener('click', () => select(i)));
  gallery.querySelector('[data-enlarge]').addEventListener('click', () => { select(index); dialog.showModal(); });
  dialog.querySelector('[data-close]').addEventListener('click', () => dialog.close());
  dialog.querySelector('[data-next]').addEventListener('click', () => select(index + 1));
  dialog.querySelector('[data-prev]').addEventListener('click', () => select(index - 1));
  dialog.addEventListener('keydown', event => {
    if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') { event.preventDefault(); select(index + (event.key === 'ArrowRight' ? 1 : -1)); }
  });
  dialog.addEventListener('click', event => { if (event.target === dialog) dialog.close(); });
  select(0);
})();
