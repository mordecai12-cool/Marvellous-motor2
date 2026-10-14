(() => {
  const token = document.cookie.split('; ').find(x => x.startsWith('mm-csrf='))?.split('=')[1];
  function protect(form) {
    if (form.method.toLowerCase() !== 'post' || !token || form.elements.namedItem('csrf-token')) return;
    const field = document.createElement('input');field.type='hidden';field.name='csrf-token';field.value=token;form.append(field);
  }
  document.querySelectorAll('form').forEach(protect);
  document.addEventListener('submit', event => { if (event.target instanceof HTMLFormElement) protect(event.target); },true);
})();
