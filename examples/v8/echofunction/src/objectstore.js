(subject, payload) => {
  this.hostServices.objectStore.put('hello', payload);
  this.hostServices.objectStore.delete('hello');

  this.hostServices.objectStore.put('hello2', payload);
  return {
    list: this.hostServices.objectStore.list(),
    hello2: this.hostServices.objectStore.get('hello2')
  }
};
