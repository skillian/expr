See the MapperBuilder type.  While it is possible to implement
mapping from one type to another without generic methods (only generic
functions), the BuildMapper implementation loses its type parameter
so it could not create a mapper implementation dynamically.  This would
be fixed if something like .NET's MakeGenericType existed.
