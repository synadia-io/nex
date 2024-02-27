(subject, payload) => {
  this.hostServices.messaging.publish('hello.world', payload);

  var req;
  var reqEx;

  var reqMany;
  var reqManyEx;

  try {
    req = this.hostServices.messaging.request('hello.world.request', payload);
  } catch (e) {
    reqEx = e;
  }

  try {
    reqMany = this.hostServices.messaging.requestMany('hello.world.request.many', payload);
  } catch (e) {
    reqManyEx = e;
  }

  return {
    'hello.world.request': req,
    'hello.world.request.ex': reqEx,
    'hello.world.request.many': reqMany,
    'hello.world.request.many.ex': reqManyEx
  }
};
