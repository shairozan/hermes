You are operating as a developer and consultant for Hermes, a proprietary command proxy called Hermes. The goal is to prioritize execution of complex PKPD (or any binary honestly) requests via:

1. a gRPC proxy that supports bi-directional streaming of STDERR/STDOUT as well as collecting request defined artifacts
2. a gRPC proxy that is capable of running in _file mode only_ to allow for the retrieval of files from _that container or volume_
3. a gRPC proxy that can operate as a _scheduler_ to fan requests out (such as sbatch) to k8s batch jobs 

The details of this design are in `design.md`. Please refer to this whenever considering implementation details. If anything _changes_ or is added in discussions with the user, please make sure to update `design.md`

# Documentation
All markdown documentation files should be organized in the `/docs` directory:
* General documentation goes in `/docs`
* Validation-related documents (requirements, validation status, test plans) go in `/docs/validation`
  * `REQUIREMENTS.md` - Functional requirements for CFR 21 Part 11 compliance
  * `VALIDATION_STATUS.md` - Current validation test status and GxP readiness
  * Other validation artifacts as needed

# Language
The server and clients should all be written in golang (go)

## Tenets
* Do not use `Make`. Use `mage` instead to keep everything go and clean
* Orthognal achitecture. Everything is built at the top and injected into lower layers
* Do not use `pkg` as a directory. This is an old idiom that has been rejected by the community
* Top level or exportable components should just be at the top level of the repository
* The command should use Cobra and Viper
   * Do _NOT_ use `init`. Instead have packages per cmd that have a `Command(...deps) (*cobra.Command, error)` signature. 
   * Each cobra command should have a private `attributes(c *cobra.Command)` function that applies their flags. 
   * After flag defintion, viper should be bound to the flagset `_ = viper.BindPFlags(c.Flags())`
   * The `Command` function is responsible for applying attributes (and mounting any sub commands)
   * Each function should use a `PreRunE` **OR** a `PersistentPreRunE` should be applied to provide a Struct representation of the configuration. Nothing should ever call directly to viper. Instead that function should be unmarshalling it onto a struct

# CFR 21 Part 11 Compliance
This product will be used in GxP settings, so we'll need to be capable of validating this (preferably by GAMP standards). As such, when _reading_ `design.md` let us establish a core set of requirements from a functionality standpoint. This is _an ongoing_ endeavor. Whenever we make changes, we should know what requirement that supports and if none exist, we should create that requirement

All validation documentation is maintained in `/docs/validation/`:
* Requirements should be documented in `/docs/validation/REQUIREMENTS.md`
* Validation status tracked in `/docs/validation/VALIDATION_STATUS.md`
* Requirements use the format REQ-{CATEGORY}-{ID} (e.g., REQ-AUD-LOC-003)

## Testing
Testing should be done with metatdata that _MAY_ correlate it back to a requirement. Not all tests will (Such as raw unit tests), but if we are producing a test that _PROVES_ or _SATISFIES_ a requirement, it should be mappable back to that requirement by its requirement ID. This allows us to generate a validation github action that runs through all of those tests for release candidates, and allows us a quick glance of "Did this work or not". 

## Boundaries
Because Hermes is essentially a command proxy, our boundaries for _most_ testing should limit to "Are we calling the correct thing". The places we will need to step over that are where we _require content_ from the execution. Since htat's part of the request we _need_ to be able to show that working. That is more of an integration or black box test, but should be all orchestratable by just adding a script into the container to create various files, having it called by the proxy, and requesting those files at termination