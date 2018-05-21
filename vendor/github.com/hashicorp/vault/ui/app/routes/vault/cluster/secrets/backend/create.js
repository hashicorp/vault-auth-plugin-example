import Ember from 'ember';
import EditBase from './secret-edit';
import KeyMixin from 'vault/models/key-mixin';

var SecretProxy = Ember.Object.extend(KeyMixin, {
  store: null,

  toModel() {
    return this.getProperties('id', 'secretData', 'backend');
  },

  createRecord(backend) {
    let modelType = 'secret';
    let backendModel = this.store.peekRecord('secret-engine', backend);
    if (backendModel.get('type') === 'kv' && backendModel.get('options.version') === 2) {
      modelType = 'secret-v2';
    }
    return this.store.createRecord(modelType, this.toModel());
  },
});

export default EditBase.extend({
  createModel(transition, parentKey) {
    const { backend } = this.paramsFor('vault.cluster.secrets.backend');
    const modelType = this.modelType(backend);
    if (modelType === 'role-ssh') {
      return this.store.createRecord(modelType, { keyType: 'ca' });
    }
    if (modelType !== 'secret' && modelType !== 'secret-v2') {
      return this.store.createRecord(modelType);
    }
    const key = transition.queryParams.initialKey || '';
    const model = SecretProxy.create({
      initialParentKey: parentKey,
      store: this.store,
    });

    if (key) {
      // have to set this after so that it will be
      // computed properly in the template (it's dependent on `initialParentKey`)
      model.set('keyWithoutParent', key);
    }
    return model;
  },

  model(params, transition) {
    const parentKey = params.secret ? params.secret : '';
    return Ember.RSVP.hash({
      secret: this.createModel(transition, parentKey),
      capabilities: {},
    });
  },
});
