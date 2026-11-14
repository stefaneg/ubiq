# The Server enforces the Writer Rule itself

The Server decides who may make a Change by matching one claim in the proxy's signed assertion against the Writer
Rule in the Deployment's configuration, a claim name and accepted values, `groups` by default; a Person whose
assertion lacks the claim or a matching value is a Reader, and a Reader's call to a write tool is refused with the
claim and values named. The proxy can deny write tools in its own policy as a second gate, but the spec does not rely
on it: Pomerium Core's group criterion needs its Enterprise directory sync, its tool denial leaves denied tools listed
and says nothing the Domain Model can explain, and it is absent in the dev direct mode. The rule lives in the
Deployment, not the repository, because the Server reads the Domain Model from the open Batch and content must not
govern its own authorisation.
